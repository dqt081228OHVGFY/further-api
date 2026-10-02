"""Integration-only fixtures. Never used by the 3000 preview or deployment."""
from pathlib import Path
import json
import sqlite3
import time

project = Path(__file__).resolve().parent.parent
qa_dir = project / ".agent-work" / "local-preview" / "qa-20261001"
database = (qa_dir / "one-api.db").resolve()
if database.parent != qa_dir.resolve() or not database.exists():
    raise RuntimeError("Only the existing isolated QA database may be seeded")
with sqlite3.connect(database, timeout=30) as connection:
    # Mirrors the test-only payment configuration used by server unit tests.
    # No real terms confirmation endpoint, payment provider or upstream is called.
    options = {
        "payment_setting.compliance_confirmed": "true",
        "payment_setting.compliance_terms_version": "v1",
        "payment_setting.compliance_confirmed_at": "0",
        "payment_setting.compliance_confirmed_by": "0",
        "payment_setting.compliance_confirmed_ip": "isolated-qa-fixture",
    }
    for key, value in options.items():
        connection.execute("INSERT INTO options(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", (key,value))
    users = dict(connection.execute("SELECT username,id FROM users WHERE username IN ('reviewadmin','reviewuser')"))
    now = int(time.time())
    for index,(name, user_id) in enumerate(users.items(), start=1):
        request_id = "qa-fixture-usage-" + name
        if connection.execute("SELECT 1 FROM logs WHERE request_id=?", (request_id,)).fetchone():
            continue
        connection.execute('''INSERT INTO logs(user_id,created_at,type,content,username,token_name,model_name,quota,prompt_tokens,completion_tokens,use_time,is_stream,channel_id,channel_name,token_id,"group",ip,request_id,other)
          VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)''', (user_id,now,2,"Isolated integration fixture",name,"QA fixture application","qa-model",index*500,index*100,index*40,2,0,0,"",9000+index,"default","127.0.0.1",request_id,json.dumps({"status_code":200,"duration_ms":2000})))
    if "reviewuser" in users:
        connection.execute("UPDATE users SET quota=CASE WHEN quota<750000 THEN 750000 ELSE quota END WHERE id=?", (users["reviewuser"],))
    connection.commit()
print("Seeded isolated QA configuration and marked integration log fixtures")
