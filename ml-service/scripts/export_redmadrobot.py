"""Export redmadrobot-rnd/rubert-base-pii-ner to ONNX and quantize to dynamic INT8.

Pipeline:
1. Export the FP32 model to ONNX with dynamic sequence length.
2. Quantize MatMul/Gemm ops to QInt8 with per_channel=True using
   onnxruntime.quantization (dynamic quantization, no calibration data needed).

The quantized model is intended for onnxruntime CPUExecutionProvider only.
"""
import os

import torch
from transformers import AutoModelForTokenClassification, AutoTokenizer

BASE_DIR = os.path.join(os.path.dirname(__file__), "..", "models")
MODEL_DIR = os.path.join(BASE_DIR, "redmadrobot-rubert-pii-ner")
FP32_PATH = os.path.join(BASE_DIR, "redmadrobot-rubert-pii-ner-fp32.onnx")
INT8_PATH = os.path.join(BASE_DIR, "redmadrobot-rubert-pii-ner-int8.onnx")


def export_fp32() -> None:
    model_dir = os.path.abspath(MODEL_DIR)
    tokenizer = AutoTokenizer.from_pretrained(model_dir)
    model = AutoModelForTokenClassification.from_pretrained(model_dir).eval()

    dummy = tokenizer("Иванов Пётр Сергеевич, паспорт 45 11 123456", return_tensors="pt")

    torch.onnx.export(
        model,
        (dummy["input_ids"], dummy["attention_mask"], dummy["token_type_ids"]),
        os.path.abspath(FP32_PATH),
        input_names=["input_ids", "attention_mask", "token_type_ids"],
        output_names=["logits"],
        dynamic_axes={
            "input_ids": {0: "batch", 1: "seq"},
            "attention_mask": {0: "batch", 1: "seq"},
            "token_type_ids": {0: "batch", 1: "seq"},
            "logits": {0: "batch", 1: "seq"},
        },
        opset_version=14,
        do_constant_folding=True,
    )
    print(f"Exported FP32 ONNX to {FP32_PATH}")


def quantize_int8() -> None:
    from onnxruntime.quantization import QuantType, quantize_dynamic

    quantize_dynamic(
        model_input=os.path.abspath(FP32_PATH),
        model_output=os.path.abspath(INT8_PATH),
        weight_type=QuantType.QInt8,
        per_channel=True,
        reduce_range=False,
        op_types_to_quantize=["MatMul", "Gemm"],
        extra_options={"EnableSubgraph": True},
    )
    print(f"Exported INT8 ONNX to {INT8_PATH}")


def main() -> None:
    export_fp32()
    quantize_int8()


if __name__ == "__main__":
    main()