# Scheduler

Scheduler manages a job queue and a maximum number of goroutines to handle jobs
submitted to that job queue. It relies on `golang.org/x/net/context`, setup by
Tasks being submitted to it to ensure that the scheduler doesn't deadlock on
jobs that never return.

This came out of a need to carefully control the number of parallel tasks we're
executing on the Opsee bastion software. We don't want a buggy check scheduler
to have the ability to either DDoS customer services or inadvertantly spin the
bastion out of control by launching more and more goroutines.

## TODO

* Add a nice shutdown mechanism that kills all currently executing and pending
  jobs and cancels their contexts, maybe?
