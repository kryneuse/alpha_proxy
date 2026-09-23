"""V2 transport validation, all-type mapping, overload and deadline behavior."""
from dataclasses import replace
from concurrent.futures import ThreadPoolExecutor

import grpc
import pytest

from ml_service import proto as pb
from ml_service.config import Config
from ml_service.core.adaptive import Overloaded, RequestCancelled
from ml_service.core.types import BatchResult, ChunkResult, ChunkAudit, Entity
from ml_service.core.v14_types import TYPES
from ml_service.server.service import PIIDetectorService, _TARGET_TO_PROTO


class Ctx:
    def __init__(self): self.aborted=None; self.trailing=None
    def abort(self,code,details):
        self.aborted=code
        raise RuntimeError(code)
    def is_active(self): return True
    def time_remaining(self): return 1.0
    def set_trailing_metadata(self,value): self.trailing=dict(value)


class Detector:
    def __init__(self,error=None): self.error=error
    def detect_batch_v2(self,batch_id,offset_unit,chunks,**kwargs):
        if self.error: raise self.error()
        return BatchResult(batch_id,'test',offset_unit,[ChunkResult(cid,
            entities=[Entity(typ,0,1,1.0) for typ in TYPES],
            audit=ChunkAudit(cid,backend='spacy_sm')) for cid,original,gate in chunks])


def request():
    return pb.DetectBatchV2Request(batch_id='b',offset_unit=pb.OffsetUnit.OFFSET_UNIT_UNICODE_CODE_POINTS,
        chunks=[pb.ChunkV2(chunk_id='c',original_text='Иван',gate_text=' '*8)])


def test_all_25_types_remain_distinct_and_route_metadata_present():
    service=PIIDetectorService(Detector(),Config())
    ctx=Ctx(); result=service.DetectBatchV2(request(),ctx)
    assert len(result.results[0].entities)==25
    assert len({e.type for e in result.results[0].entities})==25
    assert ctx.trailing['x-ml-spacy-chunks']=='1'
    assert result.results[0].error_code==pb.ChunkErrorCode.CHUNK_ERROR_CODE_NONE


@pytest.mark.parametrize('mutation',[
    lambda r:setattr(r,'batch_id',''),
    lambda r:r.ClearField('chunks'),
    lambda r:r.chunks.add().CopyFrom(r.chunks[0]),
    lambda r:r.chunks[0].ClearField('gate_text'),
    lambda r:setattr(r.chunks[0],'gate_text','x'),
    lambda r:setattr(r,'offset_unit',0),
])
def test_invalid_v2_rejected(mutation):
    r=request(); mutation(r); ctx=Ctx()
    with pytest.raises(RuntimeError): PIIDetectorService(Detector(),Config()).DetectBatchV2(r,ctx)
    assert ctx.aborted==grpc.StatusCode.INVALID_ARGUMENT


def test_batch_size_bound():
    r=request()
    for i in range(32):r.chunks.add(chunk_id=str(i),original_text='a',gate_text='a')
    ctx=Ctx()
    with pytest.raises(RuntimeError):PIIDetectorService(Detector(),Config()).DetectBatchV2(r,ctx)
    assert ctx.aborted==grpc.StatusCode.INVALID_ARGUMENT


@pytest.mark.parametrize('error,status',[(Overloaded,grpc.StatusCode.RESOURCE_EXHAUSTED),(RequestCancelled,grpc.StatusCode.CANCELLED)])
def test_failures_are_rpc_errors_not_clean(error,status):
    ctx=Ctx()
    with pytest.raises(RuntimeError): PIIDetectorService(Detector(error),Config()).DetectBatchV2(request(),ctx)
    assert ctx.aborted==status


def test_v1_explicitly_rejected():
    ctx=Ctx()
    with pytest.raises(RuntimeError):PIIDetectorService(Detector(),Config()).DetectBatch(pb.DetectBatchRequest(),ctx)
    assert ctx.aborted==grpc.StatusCode.FAILED_PRECONDITION


def test_unknown_entity_is_error_not_silent_loss():
    service=PIIDetectorService(Detector(),Config())
    result=service._to_proto_result(ChunkResult('c',entities=[Entity('BAD',0,1,1)]))
    assert not result.entities
    assert result.error_code==pb.ChunkErrorCode.CHUNK_ERROR_CODE_MODEL_ERROR


def test_real_grpc_transport():
    server=grpc.server(ThreadPoolExecutor(max_workers=2))
    pb.add_PIIDetectorServicer_to_server(PIIDetectorService(Detector(),Config()),server)
    port=server.add_insecure_port('127.0.0.1:0')
    server.start()
    try:
        with grpc.insecure_channel(f'127.0.0.1:{port}') as channel:
            result,call=pb.PIIDetectorStub(channel).DetectBatchV2.with_call(request(),timeout=2)
            assert len(result.results[0].entities)==25
            assert dict(call.trailing_metadata())['x-ml-spacy-chunks']=='1'
    finally:
        server.stop(0).wait()
