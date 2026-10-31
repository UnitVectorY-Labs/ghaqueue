[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Work In Progress](https://img.shields.io/badge/Status-Work%20In%20Progress-yellow)](https://guide.unitvectorylabs.com/bestpractices/status/#work-in-progress) 

# ghaqueue

Command-line observability for GitHub Actions queues across repositories, workflows, and runners.

See the [documentation](https://ghaqueue.unitvectorylabs.com) for installation, configuration, examples, and API limitations.

```bash
go install github.com/UnitVectorY-Labs/ghaqueue@latest
ghaqueue --org UnitVectorY-Labs
ghaqueue pools --org UnitVectorY-Labs
ghaqueue failures --org UnitVectorY-Labs --since 24h
```
