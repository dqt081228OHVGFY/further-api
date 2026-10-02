from pathlib import Path
import shutil, json, hashlib
root=Path(r"D:\test\relayfor-rebuild")
archive=root/".agent-work"/"root-pre-gateway-archive"
archive.mkdir(parents=True, exist_ok=True)
keep={".agent-work",".playwright-cli","gateway","README.md","README.en.md","README.fr.md","README.ja.md","README.zh_CN.md","README.zh_TW.md","docs","Dockerfile","Dockerfile.dev","docker-compose.yml","docker-compose.dev.yml","compose.env.example",".dockerignore",".gitignore","makefile","LICENSE","NOTICE","THIRD-PARTY-LICENSES.md",".env.example","CLAUDE.md","VERSION","NEWAPI_RELAYFOR_COMPARISON.md"}
items=[]
for p in list(root.iterdir()):
    if p.name in keep: continue
    target=archive/p.name
    if target.exists(): raise SystemExit("archive target exists: "+str(target))
    def digest(path):
        h=hashlib.sha256()
        if path.is_file():
            h.update(path.read_bytes()); return h.hexdigest()
        for f in sorted(path.rglob("*")):
            if f.is_file(): h.update(str(f.relative_to(path)).encode()); h.update(f.read_bytes())
        return h.hexdigest()
    items.append({"name":p.name,"kind":"dir" if p.is_dir() else "file","sha256":digest(p)})
    shutil.move(str(p), str(target))
(archive/"manifest.json").write_text(json.dumps({"purpose":"Pre-gateway root source archive; never package or serve.","items":items},ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print("archived",len(items),"root entries to",archive)
