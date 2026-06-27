# Third-party notices

The contextq-server binary is built entirely from this repository and the Go
standard library.

Release bundles also contain contextq `v0.2.0`. Its source is available at
<https://github.com/norlinga/contextq>. contextq is distributed under the MIT
License included as `CONTEXTQ_LICENSE` in each bundle.

The contextq binary includes the following Go modules:

| Module | Version | License |
| --- | --- | --- |
| `github.com/gofrs/flock` | `v0.13.0` | BSD-3-Clause |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause |
| `github.com/spf13/cobra` | `v1.10.2` | Apache-2.0 |
| `github.com/spf13/pflag` | `v1.0.9` | BSD-3-Clause |
| `github.com/inconshreveable/mousetrap` | `v1.1.0` | Apache-2.0 |
| `golang.org/x/sys` | `v0.37.0` | BSD-3-Clause |

The authoritative license texts and copyright notices are included in the
`licenses/` directory of each release bundle. One copy of Apache License 2.0 covers
the Cobra and mousetrap modules; the BSD-3-Clause notices are retained separately so
their original copyright statements remain intact.
