# Release progress count plan

Use the same eight-snapshot fake timeline for both paths. The controlled baseline
reconstructs the old documented watcher plus manual run and pending reads; it is
not a replay of the historical session. Expect the observer to make eight run
reads and three pending reads, with each pending read at an explicit waiting
snapshot. Expect the reconstruction to make one watcher invocation, eight manual
run reads, and eight manual pending reads. Count output bytes and command
invocations actually produced. Add the same two independent approval checks to
both paths and report those six reads separately. No diagnostic read is needed
on this successful timeline. Do not infer HTTP request counts or elapsed-time
improvement from the counts.
