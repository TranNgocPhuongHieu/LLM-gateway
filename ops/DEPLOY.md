# LLM Gateway deployment

The gateway is deployed as the `llm-gateway` service from the external Compose
project in the parent `LLM-gateway/` directory. The deployment wrapper rebuilds
only that service with `--no-deps`.

From `LLM-gateway/`:

```sh
sh source/scripts/deploy.sh
```

The wrapper validates the Compose configuration, saves the current image as a
rollback tag, builds the image, recreates only the gateway, waits for its
healthcheck, and runs `source/scripts/smoke-test.sh`. On failure after the
recreate, it restores the previous image and runs the smoke test again. A build
failure does not change the running service.

PostgreSQL (`new-api-postgres`) and Redis (`new-api-redis`) are protected by
container-ID checks and are never recreated by the wrapper.

The smoke test checks local `/readyz`, the public quota page, and container
health. It does not send an authenticated or billable API request. Optional
endpoints requiring credentials are intentionally not probed by this script.

Useful overrides are `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`, `HEALTH_TIMEOUT`,
`BASE_URL`, `PUBLIC_URL`, and `HTTP_TIMEOUT`. Do not put credentials in these
variables or in command output.
