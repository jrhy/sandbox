# sql/driver

This directory held an experimental SQL driver/query layer with backend-agnostic table interfaces, plus an
`s3db`-backed implementation in `sql/driver/db`. In practice it reads more like a precursor/exploration of ideas
around schemas, iterators, and query evaluation that later fed into (or overlapped with) `s3db`'s design, while
`s3db` itself grew into the more complex CRDT-backed storage engine.

Status: deprecated/parked. The code is retained for reference but is not maintained after `s3db` API changes.
