gitlab-fuse
===========

A [FUSE] filesystem for interacting with [GitLab], written in Go

# Usage

```
$ gitlab-fuse <mountpoint>
```

You must also provide the following values, either via command-line options or environment variables:

- `GITLAB_PRIVATE_TOKEN` or `-token` - Your GitLab private (or application) token
- `GITLAB_URL` or `-url` - The URL to your GitLab instance (e.g. `https://gitlab.example.com/api/v3`)


[FUSE]: https://en.wikipedia.org/wiki/Filesystem_in_Userspace
[GitLab]: https://docs.gitlab.com/ce/api/
