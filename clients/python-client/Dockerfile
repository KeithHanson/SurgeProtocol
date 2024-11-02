FROM python:3.9-slim

WORKDIR /app

COPY . .
RUN pip install -r requirements.txt
ENTRYPOINT ["bash", "run.sh"]