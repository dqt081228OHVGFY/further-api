from pathlib import Path
import shutil
src=Path(r"D:\test\newapi1")
dst=Path(r"D:\test\relayfor-rebuild\gateway")
if dst.exists():
    raise SystemExit("destination exists: " + str(dst))
exclude_dirs={".git","node_modules","dist","build",".cache","coverage","data","tmp"}
def ignore(dirpath, names):
    ignored=[]
    for n in names:
        p=Path(dirpath)/n
        if n in exclude_dirs or (p.is_dir() and n.startswith(".cache")):
            ignored.append(n)
            continue
        if n in {".env",".env.local",".env.production"} or p.suffix.lower() in {".db",".sqlite",".sqlite3"}:
            ignored.append(n)
    return ignored
shutil.copytree(src,dst,ignore=ignore)
print("copied " + str(src) + " -> " + str(dst))
