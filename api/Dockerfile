FROM python:3.13-slim

# uv for fast, reproducible installs. --frozen in CI once uv.lock is committed.
COPY --from=ghcr.io/astral-sh/uv:0.5 /uv /usr/local/bin/uv

WORKDIR /app

# Dependencies first: this layer is cached across code changes.
COPY pyproject.toml ./
RUN uv pip install --system --no-cache .

COPY sovereign_api ./sovereign_api
COPY migrations ./migrations

# Run unprivileged. The container needs to read code and talk to Postgres,
# nothing else.
RUN useradd --create-home --uid 10001 sovereign
USER sovereign

EXPOSE 8000

CMD ["uvicorn", "sovereign_api.app:create_app", "--factory", \
     "--host", "0.0.0.0", "--port", "8000"]
