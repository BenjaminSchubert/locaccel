![GitHub License MIT](https://img.shields.io/github/license/BenjaminSchubert/locaccel)
![Experimental status notice](https://img.shields.io/badge/Status-Experimental-red)

# Locaccel – Your Local Caching Proxy

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
- Container registries (Docker Hub, GitHub Container Registry, Google Container Registry, Quay, …)
- Generic HTTP proxy – works for Debian/Ubuntu package mirrors and any other HTTP‑based source.

Is there another registry type you'd like supported? Open an issue or
[contribute](./CONTRIBUTING.md)!

## Why Not Use Something Else?

Many tools solve part of the problem, but each comes with trade‑offs:

|Project|Strengths|Limitations|
|-------|---------|-----------|
|[Artifactory]((https://jfrog.com/artifactory/)) / [Sonatype Nexus](https://www.sonatype.com/products/sonatype-nexus-repository)|Full‑featured repository manager; handles every format Locaccel does and more|Heavyweight, complex configuration, higher resource footprint|
|[apt‑cacher‑ng](https://github.com/ashang/apt-cacher-ng)|Excellent for Debian/Ubuntu/Other distributions package caches.|No support for other ecosystems|
|[Squid](https://github.com/squid-cache/squid) / [Varnish](https://varnish-cache.org/)|Highly configurable HTTP caches|Lack built‑in URL rewriting for HTTPS endpoints, making it cumbersome for many modern registries|

Locaccel aims for the sweet spot: simple setup, low resource usage, with a broad ecosystem coverage.

## Contributing

We welcome contributions! Please read our [CONTRIBUTING.md](./CONTRIBUTING.md) docs for guidelines.

## License

Locaccel is released under the MIT License. See the [LICENSE](./LICENSE) file for details.
