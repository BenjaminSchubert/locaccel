# Leveraging Locaccel with Podman

> [!NOTE]
> This documentation assumes that you are using the default locaccel configuration and that locaccel is running on the current machine. Adapt it to fit your needs.

You can also find an example setup under [./podman-example](./podman-example) which
wires everything up.

## For pulling images

See [the official documentation for registries.conf](https://github.com/containers/image/blob/main/docs/containers-registries.conf.5.md#remapping-and-mirroring-registries) for more information

The first part of using Locaccel with Podman is to configure podman to pull
from Locaccel before trying to hit the upstream registries.

There are mutliple places where you can add this configuration:

- System wide at `/etc/containers/registries.conf` or `/etc/containers/registries.conf.d/<filename>.conf`
- For a specific user at `${XDG_CONFIG_DIR:${HOME}/.config}/containers/registries.conf` or `${XDG_CONFIG_DIR:${HOME}/.config}/containers/registries.conf.d/<filename>.conf`
`${XDG_CONFIG_DIR:${HOME}/.config}/containers/registries.conf`


An example configuration then would be:

> [!WARNING]
> If you host locaccel behind an https proxy, be sure to remove `insecure = true`


```toml
[[registry]]
location = "docker.io"
mirror = [{location = "localhost:3131", insecure = true}]

[[registry]]
location = "gcr.io"
mirror = [{location = "localhost:3132", insecure = true}]

[[registry]]
location = "quay.io"
mirror = [{location = "localhost:3133", insecure = true}]

[[registry]]
location = "ghcr.io"
mirror = [{location = "localhost:3134", insecure = true}]
```

## For software running inside containers itself

Those configurations work for both building and running containers.
When building container, environments that can be configured via configuration
files can be optimized without affecting the cache key, by mounting mounts.

An example configuration would be:

Assuming a `pip.conf` in `/etc/containers/mounts/pip.conf` with the following content:

```
[global]
index-url = http://host.containers.internal:3145/simple
trusted-host = host.containers.internal
```

And a `npmrc` in `/etc/containers/mounts/npmrc` with the following content:

```
npm_config_registry=http://host.containers.internal:3144
```

And a `gemrc` in `/etc/containers/mounts/gemrc` with the following content:

```
---
:sources:
    - http://host.containers.internal:3146
```

Then, you can configure podman to mount those files automatically by creating or
or updating a file named `/etc/containers/mounts.conf` with the following:

```
/etc/containers/mounts/gemrc:/etc/gemrc
/etc/containers/mounts/npmrc:/etc/npmrc
/etc/containers/mounts/pip.conf:/etc/pip.conf
```

And, to also enable the http_proxy, you can update `/etc/containers/containers.conf`:

```toml
[engine]
env = [
    "http_proxy=http://host.containers.internal:3142",
    # You might need to add other entries there if some of the upstreams should
    # not be reached through the proxy
    "no_proxy=host.containers.internal",
]
```

### Golang

Golang cannot be handled properly in such a way, as it cannot be configured
via configuration files.

As such, for building, you can add `--env GOSUMDB --env GOPROXY` to your
`podman build` command, but note that will break cache keys and add the
variable in the final image.

For running, you can update `/etc/containers/containers.conf` to container:

```toml
[containers]
env = [
    "GOPROXY=http://host.containers.internal:3143",
    "GOSUMDB=sum.golang.org http://host.containers.internal:3143",
]
