[![GitHub release](https://img.shields.io/github/release/UnitVectorY-Labs/ghaqueue.svg)](https://github.com/UnitVectorY-Labs/ghaqueue/releases/latest) [![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Active](https://img.shields.io/badge/Status-Active-green)](https://guide.unitvectorylabs.com/bestpractices/status/#active) 

# ghaqueue

Command-line observability for GitHub Actions queues across repositories, workflows, and runners.

See the [documentation](https://ghaqueue.unitvectorylabs.com) for installation, configuration, examples, and API limitations.

```bash
go install github.com/UnitVectorY-Labs/ghaqueue@latest
ghaqueue --org UnitVectorY-Labs
ghaqueue pools --org UnitVectorY-Labs
ghaqueue failures --org UnitVectorY-Labs --since 24h
```
