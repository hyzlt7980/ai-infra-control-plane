from fastapi import FastAPI

app = FastAPI(title="timeseries-inference-service")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"service": "timeseries-inference-service", "status": "ok"}


@app.get("/readyz")
def readyz() -> dict[str, str]:
    return {"service": "timeseries-inference-service", "status": "ready"}


@app.get("/v1/ping")
def ping() -> dict[str, str]:
    return {"message": "timeseries-inference-service placeholder endpoint"}
