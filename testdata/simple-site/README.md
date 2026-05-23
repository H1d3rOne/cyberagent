# CyberAgent Simple Test Site

本目录是 MVP smoke test 用的静态 Web fixture。

启动：

```bash
python3 -m http.server 8080 --directory testdata/simple-site
```

扫描：

```bash
bin/cyberagent scan web --target http://localhost:8080 --scope localhost --out runs/simple-site --no-external-tools
```
