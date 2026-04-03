from statistics import mean
from typing import List

from fastapi import FastAPI
from pydantic import BaseModel, Field

app = FastAPI(title="timeseries-inference-service")


class InferRequest(BaseModel):
    series: List[float] = Field(..., min_length=1)


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"service": "timeseries-inference-service", "status": "ok"}


@app.get("/readyz")
def readyz() -> dict[str, str]:
    return {"service": "timeseries-inference-service", "status": "ready"}


@app.get("/v1/ping")
def ping() -> dict[str, str]:
    return {"message": "timeseries-inference-service placeholder endpoint"}


@app.post("/infer")
def infer(payload: InferRequest) -> dict:
    values = payload.series
    prediction = values[-1]
    if len(values) >= 2:
        prediction = values[-1] + (values[-1] - values[-2])

    return {
        "model_type": "timeseries",
        "input_length": len(values),
        "mean": mean(values),
        "last_value": values[-1],
        "prediction": prediction,
    }
