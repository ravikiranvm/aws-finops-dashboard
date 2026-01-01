# Simple runtime image for aws-finops-dashboard
FROM python:3.11-slim AS runtime

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /app

# Install the CLI from source
COPY . /app
RUN pip install --upgrade pip && \
    pip install --no-cache-dir .

# Let users pass AWS/Slack/env config at runtime
ENTRYPOINT ["aws-finops"]
CMD []
