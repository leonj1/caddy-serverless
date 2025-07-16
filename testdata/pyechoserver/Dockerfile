# Use an official Python runtime as a parent image
FROM python:3.9-slim

WORKDIR /app

# Copy the requirements file and install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the rest of the application code
COPY app.py .

# Make port 8080 available to the world outside this container
# This is the port uvicorn will listen on by default as configured in CMD.
# Can be overridden by PYECHOSERVER_PORT env var if app.py's __main__ block was used,
# but for direct uvicorn CMD, we set it here.
EXPOSE 8080

# Add a startup script to ensure the server is ready
RUN echo '#!/bin/sh\necho "Starting Python echo server on port 8080..."\nuvicorn app:app --host 0.0.0.0 --port 8080 --log-level info' > /start.sh && chmod +x /start.sh

# Command to run the application using the startup script
CMD ["/start.sh"]
