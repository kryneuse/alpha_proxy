"""Export LLAIMlegal/ru-legal-ner to ONNX FP32.

Produces an ONNX model with dynamic sequence length suitable for
onnxruntime CPUExecutionProvider inference.
"""
import os
import sys

import torch
from transformers import AutoModelForTokenClassification, AutoTokenizer

MODEL_DIR = os.path.join(os.path.dirname(__file__), "..", "models", "llaim-ru-legal-ner")
OUT_PATH = os.path.join(os.path.dirname(__file__), "..", "models", "llaim-ru-legal-ner.onnx")


def main() -> None:
    model_dir = os.path.abspath(MODEL_DIR)
    out_path = os.path.abspath(OUT_PATH)

    tokenizer = AutoTokenizer.from_pretrained(model_dir)
    model = AutoModelForTokenClassification.from_pretrained(model_dir).eval()

    dummy = tokenizer("ООО «Ромашка», ИНН 7701234567", return_tensors="pt")

    torch.onnx.export(
        model,
        (dummy["input_ids"], dummy["attention_mask"], dummy["token_type_ids"]),
        out_path,
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
    print(f"Exported LLAIM ONNX to {out_path}")


if __name__ == "__main__":
    main()