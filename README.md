[![CI](https://github.com/BenjaminSchubert/locaccel/actions/workflows/ci.yml/badge.svg)](https://github.com/BenjaminSchubert/locaccel/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/BenjaminSchubert/locaccel/graph/badge.svg?token=8GLYR6RCR3)](https://codecov.io/gh/BenjaminSchubert/locaccel)
![GitHub License MIT](https://img.shields.io/github/license/BenjaminSchubert/locaccel)
![Experimental status notice](https://img.shields.io/badge/Status-Experimental-red)

# Locaccel – Your Local Caching Proxy

- [Overview](#overview)
- [What It Supports](#what-it-supports)
- [Why choose locaccel?](#why-choose-locaccel)
- [Getting Started](#getting-started)
    - [Installation](#installation)
    - [Configuration](#configuration)
    - [Configuring tooling to leverage locaccel](#configuring-tooling-to-leverage-locaccel)
- [Contributing](#contributing)

## Overview

Locaccel is a lightweight caching proxy that sits between your environment
and upstream registries. By caching artifacts locally it reduces repeated
downloads, speeds up CI pipelines and local tests, and saves bandwidth.

You can even chain multiple locaccel instances, and if one is down, it will still
query upstream instead, allowing you to have a setup with multiple caches location,
with still only one cache reaching out to the Internet.

## What It Supports

- PyPI‑compatible registries
- NPM‑compatible registries
- Go module proxies
- Ruby Gems
- Container registries (Docker Hub, GitHub Container Registry, Google Container Registry, Quay, …)
- Generic HTTP proxy – works for Debian/Ubuntu package mirrors and any other HTTP‑based source.

Is there another registry type you'd like supported? Open an issue or
[contribute](./CONTRIBUTING.md)!

## Why Choose locaccel?

Many tools solve part of the problem, but each comes with trade‑offs:

|Project|Strengths|Limitations|
|-------|---------|-----------|
|[Artifactory](https://jfrog.com/artifactory/) / [Sonatype Nexus](https://www.sonatype.com/products/sonatype-nexus-repository)|Full‑featured repository manager; handles every format Locaccel does and more|Heavyweight, complex configuration, higher resource footprint|
|[apt‑cacher‑ng](https://github.com/ashang/apt-cacher-ng)|Excellent for Debian/Ubuntu/Other distributions package caches.|No support for other ecosystems|
|[Squid](https://github.com/squid-cache/squid) / [Varnish](https://varnish-cache.org/)|Highly configurable HTTP caches|Lack built‑in URL rewriting for HTTPS endpoints, making it cumbersome for many modern registries|

Locaccel aims for the sweet spot: simple setup, low resource usage, with a broad ecosystem coverage.

## Getting started

### Installation

#### Docker/Podman

locaccel is distributed as a OCI image under `ghcr.io/benjaminschubert/locaccel`.
We support the following tags:

- `latest`, which points to the latest released version
- `edge`, which represents the state of the main branch, so the version in development
- `X`, which designates a specific major version
- `X.Y`, which designates a specific major.minor version
- `X.Y.Z`, which designates a specific major.minor.patch version

You can get started by running:

```bash
docker run \
  --name locaccel \
  --volume locaccel:/cache:rw,Z \
  # For docker.io
  --publish 3131:3131 \
  # For gcr.io
  --publish 3132:3132 \
  # For quay.io
  --publish 3133:3133 \
  # For ghcr.io \
  --publish 3134:3134 \
  # For ubuntu/debian
  --publish 3142:3142 \
  # For proxy.golang.org
  --publish 3143:3143 \
  # For registry.npmjs.org
  --publish 3144:3144 \
  # For pypi.org
  --publish 3145:3145 \
  # For rubygems.org
  --publish 3146:3146 \
  ghcr.io/benjaminschubert/locaccel
```

#### Linux

You can also download [binaries](https://github.com/BenjaminSchubert/locaccel/releases/latest) directly,
published as part of the releases.

You can then install it, e.g. with systemd:

```bash
tar xvf locaccel.*.tar.gz
cp locaccel /usr/local/bin/locaccel
cp locaccel.service /etc/systemd/system/locaccel.service
useradd --system locaccel
mkdir /var/cache/locaccel
chown locaccel: /var/cache/locaccel
# Optionally create a configuration file at /etc/locaccel/locaccel.yaml
systemctl daemon-reload
systemctl start locaccel
systemctl enable locaccel
```

### Configuration

locaccel uses a file for configuration, and exposes a subset of parameters via
environment variables.

The configuration file follows this format, and has the defaults mentioned in the
example. When providing a configuration file, note that all the defaults will
still be used unless added in the configuration file, except for the proxies
themselves, which will need to be included.

```yaml
# The interface on which to expose the caches
host: localhost
cache:
    # The path in which to store the cached files
    path: _cache
    # Whether the cache is a private cache or a public one.
    # Setting it as private will allow for caching more files, but might leak
    # private information, like secrets and passwords if exposed publicly
    private: false
    # The maximum amount of space that the cache can grow to. Once reached, this
    # will trigger a cleanup of the cache, removing least recently used files.
    # Accepts a % of the partition's size, or an amount in bytes, with the
    # following units: B, K, M, G, T
    quota_high: 20%
    # The quota at which to stop cleaning when a garbage collection is triggered.
    # This will clean files until only this amount is remaining. Having it too
    # close from `quota_high` will put more pressure on the cache and lead to
    # cleaning more often.
    # See `quota_high` for acceptable values
    quota_low: 10%

log:
    # The level at which to log
    # Acceptable values are `trace`, `debug`, `info`, `warn`, `error`, `panic`
    level: info

    # The format to use for logging
    # Acceptable values are `json` and `console`, which is a logfmt format with
    # colors enabled
    format: json

# The interface and port on which to expose the admin interface, metrics and
# optionally profiling information
admin_interface: localhost:3130

# Whether to expose prometheus metrics. If enabled, metrics will be available under
# <admin_interface>/metrics
metrics: true

# Whether to enable profiling or not. If enabled, profiling endpoints will be
# available under <admin_interface>/-/pprof/
profiling: false

# Configures a list of proxies for Go modules
go_proxies:
      # The upstream Golang module proxy
    - upstream: https://proxy.golang.org
      # The path where the sumdb can be found
      sumdb_url: https://sum.golang.org/
      # The port on which to expose the cache locally
      port: 3143
      # Optionally, a list of urls pointing to optional caches, that are going
      # to be tried first before hitting the upstream. This allows for chaining
      # caches or build a mesh in order to more efficiently reduce downloads
      upstream_caches: []

npm_registries:
      # The upstream npm registry
    - upstream: https://registry.npmjs.org/
      # The scheme to use when connecting to the cache, e.g. if you set locaccel
      # behind a reverse proxy providing ssl, use https
      scheme: http
      # The port on which to expose the cache locally
      port: 3144
      # Optionally, a list of urls pointing to optional caches, that are going
      # to be tried first before hitting the upstream. This allows for chaining
      # caches or build a mesh in order to more efficiently reduce downloads
      upstream_caches: []

oci_registries:
      # The upstream oci registry
    - upstream: https://registry-1.docker.io
      # The port on which to expose the cache locally
      port: 3131
      # Optionally, a list of urls pointing to optional caches, that are going
      # to be tried first before hitting the upstream. This allows for chaining
      # caches or build a mesh in order to more efficiently reduce downloads
      upstream_caches: []
    - upstream: https://gcr.io
      port: 3132
      upstream_caches: []
    - upstream: https://quay.io
      port: 3133
      upstream_caches: []
    - upstream: https://ghcr.io
      port: 3134
      upstream_caches: []

pypi_registries:
      # The upstream pypi registry
    - upstream: https://pypi.org/
      # The CDN used to store the python packages, so that locaccel knows how
      # to replace it in the index files served
      cdn: https://files.pythonhosted.org
      # The port on which to expose the cache locally
      port: 3145
      # Optionally, a list of urls pointing to optional caches, that are going
      # to be tried first before hitting the upstream. This allows for chaining
      # caches or build a mesh in order to more efficiently reduce downloads
      upstream_caches: []

proxies:
      # The list of allowed upstream hostnames that locaccel can proxy
    - allowed_upstream:
        # Debian packages
        - deb.debian.org
        # Ubuntu packages
        - archive.ubuntu.com
        - security.ubuntu.com
      # The port on which to expose the cache locally
      port: 3142
      # Optionally, a list of urls pointing to optional caches, that are going
      # to be tried first before hitting the upstream. This allows for chaining
      # caches or build a mesh in order to more efficiently reduce downloads
      upstream_caches: []

rubygem_registries:
      # The upstream rubygem registry
    - upstream: https://rubygems.org
      # The port on which to expose the cache locally
      port: 3146
      # Optionally, a list of urls pointing to optional caches, that are going
      # to be tried first before hitting the upstream. This allows for chaining
      # caches or build a mesh in order to more efficiently reduce downloads
      upstream_caches: []
```

The environment variables override the values of the configuration file and are
the following:

- `LOCACCEL_ADMIN_INTERFACE`, to override `admin_interface`
- `LOCACCEL_CACHE_PATH`, to override `cache.path`
- `LOCACCEL_CONFIG_PATH`, to define where the configuration should be (default: `locaccel.yaml`)
- `LOCACCEL_HOST`, to override `host`
- `LOCACCEL_LOG_FORMAT`, to override `log.format`
- `LOCACCEL_LOG_LEVEL`, to override `log.level`

### Configuring tooling to leverage locaccel

#### Go

Set the following environment variables:

- `GOPROXY=<locaccel-url>:<goproxy port>`
- `GOSUMDB=sum.golang.org <locaccel-url>:<goproxy port>`

#### NPM

Set the following in your `.npmrc` or as environment variables:

- `npm_config_registry=<locaccel-url>:<npm port>`

#### PyPI (Python)

Set the following in your [pip.conf](https://pip.pypa.io/en/stable/topics/configuration/):

```
[global]
index-url = "<locaccel-url>:<pypi port>/simple"
```

Or as environment variable:

- `PIP_INDEX_URL=<locaccel-url>:<pypi port>/simple`

#### Ruby Gems

Set the following in your .gemrc:

```
---
:sources:
- <locaccel-url>:<rubygem port>
- https://rubygems.org
```

Or run:

```bash
gem sources --add <locaccel-url>:<rubygem port>
```

###### Bundler

If using bundler, you can put in `~/.bundle/config`:

```
bundle config mirror.https://rubygems.org <locaccel-url>:<rubygem port>
```

#### Apt (debian/ubuntu)

In a file like `/etc/apt/apt.conf.d/80-proxy`, add the following:

```
Acquire::http::proxy  "http://<locaccel-url>:<proxy port>/";
Acquire::https::Proxy "http://<locaccel-url>:<proxy port>/";
```

#### Podman (OCI)

See [the full docs](./docs/ecosystems/podman.md)


## Contributing

We welcome contributions! Please read our [CONTRIBUTING.md](./CONTRIBUTING.md) docs for guidelines.

## License

Locaccel is released under the MIT License. See the [LICENSE](./LICENSE) file for details.
