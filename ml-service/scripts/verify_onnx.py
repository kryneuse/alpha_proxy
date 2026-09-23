"""Verify ONNX exports match PyTorch reference outputs.

Compares argmax labels and softmax probabilities between the PyTorch model
and the ONNX Runtime (FP32 / INT8) inference for both models.
"""
import os
import sys

import numpy as np
import onnxruntime as ort
import torch
from transformers import AutoModelForTokenClassification, AutoTokenizer

BASE = os.path.join(os.path.dirname(__file__), "..", "models")


def _run_ort(path: str, enc) -> np.ndarray:
    so = ort.SessionOptions()
    so.intra_op_num_threads = 1
    so.inter_op_num_threads = 1
    sess = ort.InferenceSession(path, sess_options=so, providers=["CPUExecutionProvider"])
    feeds = {
        "input_ids": enc["input_ids"].numpy(),
        "attention_mask": enc["attention_mask"].numpy(),
        "token_type_ids": enc["token_type_ids"].numpy(),
    }
    return sess.run(None, feeds)[0]


def check(model_dir: str, onnx_paths: list, text: str) -> None:
    tokenizer = AutoTokenizer.from_pretrained(model_dir)
    model = AutoModelForTokenClassification.from_pretrained(model_dir).eval()
    enc = tokenizer(text, return_tensors="pt")

    with torch.no_grad():
        pt_logits = model(**enc).logits.numpy()

    pt_labels = pt_logits.argmax(-1)
    pt_probs = torch.softmax(torch.from_numpy(pt_logits), -1).numpy()

    for path in onnx_paths:
        ort_logits = _run_ort(path, enc)
        ort_labels = ort_logits.argmax(-1)
        ort_probs = np.exp(ort_logits - ort_logits.max(-1, keepdims=True))
        ort_probs = ort_probs / ort_probs.sum(-1, keepdims=True)

        label_match = (pt_labels == ort_labels).mean()
        prob_diff = np.abs(pt_probs - ort_probs).max()
        print(f"  {os.path.basename(path)}: label_match={label_match:.4f} max_prob_diff={prob_diff:.6f}")


def main() -> None:
    print("LLAIMlegal/ru-legal-ner:")
    check(
        os.path.join(BASE, "llaim-ru-legal-ner"),
        [os.path.join(BASE, "llaim-ru-legal-ner.onnx")],
        "ООО «Ромашка», ИНН 7701234567, тел. +7 999 123-45-67",
    )
    print("redmadrobot-rnd/rubert-base-pii-ner:")
    check(
        os.path.join(BASE, "redmadrobot-rubert-pii-ner"),
        [
            os.path.join(BASE, "redmadrobot-rubert-pii-ner-fp32.onnx"),
            os.path.join(BASE, "redmadrobot-rubert-pii-ner-int8.onnx"),
        ],
        "Иванов Пётр Сергеевич, паспорт 45 11 123456, тел. +7 999 123-45-67",
    )


if __name__ == "__main__":
    main()