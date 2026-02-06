# cdcfun

This directory used to contain a small CLI that read per-day case counts from an S3-backed `s3db` database
and printed rolling active case counts plus simple growth/R0 estimates. It relied on the older `github.com/jrhy/s3db`
API and a `.bucket` + `.key` file in the working directory.

Status: deprecated/parked. The code is left here for historical reference and is not maintained after the
`github.com/jrhy/s3db` API changes.
