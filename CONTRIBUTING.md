# Contributing

The easiest way to ensure the guidelines pass when making a code change is to run:

```shell
make all
```

See below for more details on each steps

## Running locally

You can run locaccel locally with live reload by using [air](https://github.com/air-verse/air), which is abstracted for you by running:

```shell
make
```

Or, without reloading:

```shell
go run cmd/locaccel/locaccel.go
```

## Testing

Locaccel uses the standard go toolchain for this, you can thus run:

```shell
# Natively
go test ./...

# Using the Makefile
make test
```

Note that `podman` is required in order to run the integration tests.

## Linting

Locaccel uses [golanci-lint](https://golangci-lint.run/) for linting.
You can replicate what CI runs by doing:

```shell
# To run the checks
make lint

# Or to auto fix problems where possible
make fix
```

## Profiling

You can natively enable go profiling by exporting `LOCACCEL_ENABLE_PROFILING=1`, which will exxpose all metrics on `http://<interface>:<admin-port>/-/pprof/`

For example:

```shell
LOCACCEL_ENABLE_PROFILING=1 go run cmd/locaccel/locaccel.go
go tool pprof -http localhost:8000 http://localhost:3130/-/pprof/allocs
```
