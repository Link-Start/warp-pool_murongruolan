# Changelog

## v0.1.5

- Fixed node mode switch language inheritance. Pull-installed nodes now save the selected language, and `node_mode.sh` reads it for later direct/WARP switches.
- Added clearer one-time token expiry warnings for Deploy Token and node mode switch commands.
- Added Cloudflare WARP disk and inode preflight checks before installing the official WARP client, with clearer recovery guidance when `apt` fails because of low disk quota.
- Fixed SSH Push deployment for non-root users by automatically using `sudo` for privileged remote operations when available.
