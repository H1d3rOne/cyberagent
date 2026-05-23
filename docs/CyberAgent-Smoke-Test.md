# CyberAgent MVP Smoke Test

版本：v0.1-dev  
日期：2026-05-23  
状态：MVP smoke test 记录

---

## 1. 靶场 fixture

```text
testdata/simple-site/index.html
testdata/simple-site/app.js
testdata/simple-site/robots.txt
testdata/simple-site/sitemap.xml
```

启动：

```bash
python3 -m http.server 18080 --directory testdata/simple-site
```

---

## 2. 验证命令

```bash
go test ./...
go vet ./...
go build -o bin/cyberagent ./cmd/cyberagent

bin/cyberagent version
bin/cyberagent doctor
bin/cyberagent plugins list
bin/cyberagent plugins doctor

bin/cyberagent scan web \
  --target http://127.0.0.1:18080 \
  --scope 127.0.0.1 \
  --profile safe \
  --out runs/smoke-simple-site \
  --no-external-tools

bin/cyberagent report runs/smoke-simple-site
bin/cyberagent replay runs/smoke-simple-site
```

---

## 3. 预期结果

```text
scan web 成功完成
run workspace 写入完整文件
report.md 可读取
replay 能看到 job.created / job.finished
plugins list 显示 approval 字段
plugins doctor 不阻塞缺失的可选外部工具
```
