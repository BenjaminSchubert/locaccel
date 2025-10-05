# Leveraging Locaccel with Podman

> [!NOTE]
> This documentation assumes that you are using the default locaccel configuration and that locaccel is running on the current machine. Adapt it to fit your needs.

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
mirror = [
    {location = "localhost:3131", insecure = true},
]

[[registry]]
location = "gcr.io"
mirror = [
    {location = "localhost:3132", insecure = true},
]

[[registry]]
location = "quay.io"
mirror = [
    {location = "localhost:3133", insecure = true},
]

[[registry]]
location = "ghcr.io"
mirror = [
    {location = "localhost:3134", insecure = true}
]
```

## For software running inside containers itself

https://github.com/containers/common/blob/main/docs/containers.conf.5.md#engine-table
