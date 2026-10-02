from pathlib import Path
import shutil
src=Path(r"D:\test\relayfor-rebuild\web")
dst=Path(r"D:\test\relayfor-rebuild\gateway\web")
if dst.exists(): shutil.rmtree(dst)
exclude_dirs={"node_modules","dist",".tanstack","coverage",".cache"}
def ignore(dirpath,names):
    return [n for n in names if n in exclude_dirs or n.startswith(".cache")]
shutil.copytree(src,dst,ignore=ignore)
print("copied RelayFor-integrated web into gateway/web")
