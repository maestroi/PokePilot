# Model worker concurrency

Model deployments may set `max_parallel_workers` to limit concurrent farm runs sharing an inference deployment. The safe default is one worker, preserving per-run decode throughput for paired model comparisons. Runs above a deployment's capacity remain queued while free deployments can continue leasing workers.

Paired experiments may choose a worker count up to each deployment's configured ceiling. Concurrency is included in the comparable run identity so differently loaded arms are not reported as directly comparable.
