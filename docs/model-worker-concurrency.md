# Model worker concurrency

Model deployments may set `max_parallel_workers` to limit concurrent farm runs sharing an inference deployment. The safe default is one worker, preserving per-run decode throughput for paired model comparisons. Runs above a deployment's capacity remain queued while free deployments can continue leasing workers.

The operator deployments panel can change that cap live (`PATCH /v1/models/{id}`). Postgres (or the JSON registry file) persists it, so a wall restart keeps the new value. Farm runs spend most of their time emulating rather than waiting on the GPU, so one inference process can interleave several runners without `--parallel` slots; keep the cap at 1 when you want a fair 1v1 benchmark.

Paired experiments may choose a worker count up to each deployment's configured ceiling. Concurrency is included in the comparable run identity so differently loaded arms are not reported as directly comparable.
