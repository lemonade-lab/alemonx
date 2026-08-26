# System dependency sources

ALemonX treats a dependency source as a system configuration transaction, not as a download-speed toggle. The current MVP is read-only on every platform: it can check mirror metadata, audit backups, and clean up ALemonX legacy files, but it never adds, switches, or restores system repositories automatically.

## DNF/YUM: never add a second BaseOS/AppStream set

CentOS Stream BaseOS and AppStream are one system package set. Enabling a mirror under new repository IDs (such as `alemonx-baseos`) makes DNF solve dependencies across both the system repositories and the mirror. Metadata that is not perfectly aligned can then select conflicting builds, including Cockpit components.

DNF/YUM therefore currently offers read-only metadata checks only, and quarantines the legacy appended-repository mode. If `/etc/yum.repos.d/alemonx-mirror.repo` exists, the UI can back up and remove it after confirmation only when it carries the ALemonX ownership marker; a same-name unmarked file is never removed. Do not hide a conflict from this legacy mode with `--allowerasing`, `--nobest`, or `--skip-broken`.

Any future DNF/YUM write support must be a full replacement transaction: modify only positively identified official CentOS Stream entries, preserve repository IDs, `gpgkey`, `enabled`, modules, and third-party repositories, back up every touched file byte-for-byte, and restore on index refresh failure. Systems that cannot be identified precisely remain read-only.

## APT and other platforms

The legacy APT drop-in and all other automatic write paths are disabled. A marked fixed legacy file can be backed up and removed after confirmation; backups are audit records only in the MVP and cannot be written back, because that could re-enable an incompatible repository.
