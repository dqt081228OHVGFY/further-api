from pathlib import Path
import hashlib
base=Path(r"D:\test\newapi1"); cur=Path(r"D:\test\relayfor-rebuild")
exclude={".git",".agent-work",".playwright-cli","node_modules","dist","data"}
def files(root):
    out={}
    for p in root.rglob("*"):
        if not p.is_file(): continue
        rel=p.relative_to(root)
        if any(part in exclude for part in rel.parts): continue
        out[str(rel).replace("\\","/")]=p
    return out
a=files(base); b=files(cur)
only=sorted(set(b)-set(a)); missing=sorted(set(a)-set(b)); diff=[]
for k in sorted(set(a)&set(b)):
    if hashlib.sha256(a[k].read_bytes()).digest()!=hashlib.sha256(b[k].read_bytes()).digest(): diff.append(k)
print("only relay",len(only)); print("\n".join(only[:500])); print("missing from relay",len(missing)); print("\n".join(missing[:200])); print("diff",len(diff)); print("\n".join(diff[:500]))
