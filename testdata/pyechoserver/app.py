from fastapi import FastAPI, Request, HTTPException
from fastapi.responses import JSONResponse
import uvicorn
import os

app = FastAPI()

@app.get("/health")
async def health():
    return {"status": "ok"}

@app.post("/{path:path}")
async def echo(request: Request, path: str = ""):
    try:
        headers = dict(request.headers)
        body = await request.body()
        
        response_data = {
            "headers": headers,
            "body": body.decode('utf-8') # Assuming UTF-8, adjust if other encodings are expected
        }
        return JSONResponse(content=response_data, status_code=200)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

# This block is for local development/testing if you run `python app.py`
# In production with Docker, uvicorn will be called directly.
if __name__ == "__main__":
    port = int(os.environ.get("PYECHOSERVER_PORT", 8080))
    uvicorn.run(app, host="0.0.0.0", port=port)
