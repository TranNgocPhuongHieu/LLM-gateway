#!/usr/bin/env python3
"""Small, single-host edge gate for an approved maintenance window.

Run this on the same host as Cloudflared and route only llm.trnhieu.dev to it.
It is deliberately not enabled by Compose or Cloudflared in this repository.
Payment callbacks are stored in SQLite before acknowledgement when the backend
is paused/unreachable; leave replays them with a stable event key.
"""
import argparse, hashlib, hmac, http.client, json, os, select, socket, sqlite3, sys, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HOSTNAME = "llm.trnhieu.dev"
PAYMENT_PATHS = ("/api/payment/sepay/ipn", "/api/payment/sepay/webhook")
MAX_SPOOL_BODY = 1024 * 1024
MAX_REQUEST_BODY = 8 * 1024 * 1024
MAX_CONCURRENT = 256
CLIENT_TIMEOUT = 15
WS_IDLE_TIMEOUT = 600

class State:
    def __init__(self, db):
        self.db = sqlite3.connect(db, check_same_thread=False)
        self.db.execute("create table if not exists spool (event_key text primary key, path text not null, body blob not null, headers text not null, created integer not null, delivered integer not null default 0)")
        self.db.execute("create table if not exists gate_state (name text primary key, value integer not null)")
        self.db.commit()
        rows=dict(self.db.execute("select name,value from gate_state"))
        self.lock = threading.RLock(); self.draining = bool(rows.get("draining", 0)); self.payment_paused = bool(rows.get("payment_paused", 0)); self.active = 0
        self.slots = threading.BoundedSemaphore(int(os.environ.get("EDGE_MAX_CONCURRENT", MAX_CONCURRENT)))
    def set_flag(self, name, value):
        self.db.execute("insert into gate_state(name,value) values(?,?) on conflict(name) do update set value=excluded.value", (name, int(value)))
        self.db.commit()
    def add(self, path, body, headers):
        key = hashlib.sha256(path.encode()+b"\0"+body).hexdigest()
        with self.lock:
            self.db.execute("insert or ignore into spool(event_key,path,body,headers,created) values(?,?,?,?,?)", (key,path,body,json.dumps(headers),int(time.time())))
            self.db.commit()
        return key
    def pending(self):
        with self.lock: return self.db.execute("select event_key,path,body,headers from spool where delivered=0 order by created,event_key").fetchall()

class Gate(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.0"
    def state(self): return self.server.state
    def allowed_host(self): return self.headers.get("Host", "").split(":",1)[0].lower() == HOSTNAME
    def control(self, action):
        s=self.state()
        if action=="payment-leave" and not replay(s):
            self.send_response(503); self.end_headers(); return
        with s.lock:
            if action=="enter": s.draining=True; s.set_flag("draining", True)
            elif action=="leave": s.draining=False; s.set_flag("draining", False)
            elif action=="payment-enter": s.payment_paused=True; s.set_flag("payment_paused", True)
            elif action=="payment-leave": s.payment_paused=False; s.set_flag("payment_paused", False)
            else: pass
            active=s.active; draining=s.draining; paused=s.payment_paused
        if action == "payment-check":
            code = 200 if paused else 409
        else:
            code=202 if action in ("enter", "leave", "payment-enter", "payment-leave") else (200 if active==0 else 409)
        self.send_response(code); self.send_header("Content-Type","application/json"); self.end_headers()
        self.wfile.write(json.dumps({"active":active,"draining":draining,"payment_paused":paused,"pending":len(s.pending())}).encode())
    def do_POST(self):
        if self.path.startswith("/__gate/"):
            token=os.environ.get("EDGE_GATE_TOKEN","")
            supplied=self.headers.get("X-Edge-Gate-Token","")
            if not token or not hmac.compare_digest(supplied, token): self.send_error(404); return
            self.control(self.path.rsplit("/",1)[-1]); return
        self.proxy()
    def do_GET(self): self.proxy()
    def proxy(self):
        s=self.state(); payment=self.path.split("?",1)[0] in PAYMENT_PATHS
        if not self.allowed_host(): self.send_error(421); return
        if not s.slots.acquire(blocking=False): self.send_error(503, "edge concurrency limit"); return
        with s.lock:
            if s.draining and not payment:
                s.slots.release(); self.send_error(503, "maintenance"); return
            s.active += 1
        try:
            if self.headers.get("Upgrade", "").lower() == "websocket":
                self.proxy_websocket()
                return
            length=int(self.headers.get("Content-Length","0"));
            if length < 0 or length > MAX_REQUEST_BODY:
                self.send_error(413, "request body too large"); return
            body=self.rfile.read(length) if length else b""
            if payment and length > MAX_SPOOL_BODY:
                self.send_error(413, "payment payload too large"); return
            headers={k:v for k,v in self.headers.items() if k.lower() not in ("host","content-length","connection")}
            if payment and s.payment_paused:
                key=s.add(self.path.split("?",1)[0],body,headers)
                self.send_response(202); self.send_header("Content-Type","application/json"); self.end_headers(); self.wfile.write(json.dumps({"accepted":True,"event_key":key}).encode()); return
            try:
                conn=http.client.HTTPConnection(os.environ.get("EDGE_BACKEND_HOST","127.0.0.1"),int(os.environ.get("EDGE_BACKEND_PORT","3000")),timeout=CLIENT_TIMEOUT)
                conn.request(self.command,self.path,body=body,headers=headers); resp=conn.getresponse()
                self.send_response(resp.status)
                for k,v in resp.getheaders():
                    if k.lower() not in ("connection","transfer-encoding"): self.send_header(k,v)
                self.end_headers()
                while True:
                    chunk=resp.read(65536)
                    if not chunk: break
                    self.wfile.write(chunk); self.wfile.flush()
                conn.close()
            except (OSError, http.client.HTTPException):
                if payment:
                    key=s.add(self.path.split("?",1)[0],body,headers); self.send_response(202); self.end_headers(); self.wfile.write(key.encode())
                else: self.send_error(503, "backend unavailable")
        finally:
            with s.lock: s.active -= 1
            s.slots.release()
    def proxy_websocket(self):
        """Tunnel an already-authenticated HTTP WebSocket upgrade to localhost."""
        host=os.environ.get("EDGE_BACKEND_HOST","127.0.0.1"); port=int(os.environ.get("EDGE_BACKEND_PORT","3000"))
        upstream=socket.create_connection((host, port), timeout=CLIENT_TIMEOUT)
        try:
            upstream.settimeout(CLIENT_TIMEOUT)
            request=(f"{self.command} {self.path} HTTP/1.1\r\n" + "".join(f"{k}: {v}\r\n" for k,v in self.headers.items()) + "\r\n").encode()
            upstream.sendall(request)
            response=b""
            while b"\r\n\r\n" not in response and len(response) < 65536:
                chunk=upstream.recv(4096)
                if not chunk: break
                response += chunk
            self.connection.sendall(response)
            if not response.startswith(b"HTTP/1.1 101"):
                return
            upstream.settimeout(None); self.connection.settimeout(None); self.close_connection=True
            sockets=[self.connection, upstream]
            while True:
                readable, _, _ = select.select(sockets, [], [], int(os.environ.get("EDGE_WS_IDLE_TIMEOUT", WS_IDLE_TIMEOUT)))
                if not readable: raise TimeoutError("websocket idle timeout")
                for source in readable:
                    data=source.recv(65536)
                    if not data: return
                    target=upstream if source is self.connection else self.connection
                    target.sendall(data)
        finally:
            upstream.close()
    def log_message(self,*a): pass

def replay(s):
    for key,path,body,raw in s.pending():
        try:
            headers=json.loads(raw); conn=http.client.HTTPConnection(os.environ.get("EDGE_BACKEND_HOST","127.0.0.1"),int(os.environ.get("EDGE_BACKEND_PORT","3000")),timeout=CLIENT_TIMEOUT)
            conn.request("POST",path,body=body,headers=headers); status=conn.getresponse().status; conn.close()
            if 200 <= status < 300:
                with s.lock: s.db.execute("update spool set delivered=1 where event_key=?",(key,)); s.db.commit()
            else: return False
        except (OSError,http.client.HTTPException): return False
    return True

def main():
    p=argparse.ArgumentParser(); p.add_argument("action",choices=("serve","enter","check","leave","payment-enter","payment-check","payment-leave")); p.add_argument("--db",default=os.environ.get("EDGE_GATE_DB","/var/lib/edge-gate/spool.db")); p.add_argument("--port",type=int,default=int(os.environ.get("EDGE_GATE_PORT","8080"))); a=p.parse_args()
    os.makedirs(os.path.dirname(a.db) or ".",exist_ok=True); s=State(a.db)
    if a.action=="serve":
        server=ThreadingHTTPServer(("127.0.0.1",a.port),Gate); server.state=s; server.serve_forever()
    else:
        token=os.environ.get("EDGE_GATE_TOKEN",""); action=a.action
        wire_action=action
        conn=http.client.HTTPConnection("127.0.0.1",a.port,timeout=3); conn.request("POST","/__gate/"+wire_action,headers={"Host":HOSTNAME,"X-Edge-Gate-Token":token}); r=conn.getresponse(); print(r.read().decode()); sys.exit(0 if r.status<300 else 1)
if __name__=="__main__": main()
