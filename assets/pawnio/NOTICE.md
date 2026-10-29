# PawnIO notices

## `LpcACPIEC.bin`

PawnIO module used as a runtime data file, not linked into ECC's code.

- Source: PawnIO.Modules 0.2.11
  <https://github.com/namazso/PawnIO.Modules/releases/tag/0.2.11>
- License: GNU Lesser General Public License v2.1 (see `COPYING`)
- SHA-256: `c38fd116e7aff4d1fdb0a494e296be0a6708e5a22fc72f14587442fb7f8f7906`

## `PawnIO_setup.exe`

Official PawnIO Setup 2.2.0, bundled unmodified so the ECC installer can
install the signed kernel driver when it is missing.

- Source: <https://github.com/namazso/PawnIO.Setup/releases/tag/2.2.0>
- Project: <https://pawnio.eu/>
- License: GNU General Public License v2.0 (PawnIO)
- SHA-256: `1f519a22e47187f70a1379a48ca604981c4fcf694f4e65b734aaa74a9fba3032`
- The PawnIO Setup binary states that it can be redistributed unmodified.

Command used by the installer:

```text
PawnIO_setup.exe -install -silent
```

The installer skips this step when the `PawnIO` service already exists.
ECC does not uninstall PawnIO during its own uninstall, because other
applications may share it.
