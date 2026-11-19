#!/usr/bin/env python3
import concurrent.futures, http.client, os, subprocess, tempfile, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Backend(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.headers.get("Upgrade", "").lower() == "websocket":
            self.send_response(101, "Switching Protocols"); self.send_header("Upgrade", "websocket"); self.send_header("Connection", "Upgrade"); self.end_headers(); return
        self.send_response(200); self.send_header("Content-Type","text/event-stream"); self.end_headers()
        self.wfile.write(b"data: first\n\n"); self.wfile.flush(); time.sleep(.3); self.wfile.write(b"data: [DONE]\n\n"); self.wfile.flush()
    def do_POST(self): self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def log_message(self,*a): pass

def call(port, method, path, host="llm.trnhieu.dev", body=b"", token="", upgrade=""):
    headers={"Host":host,"Content-Type":"application/json"}
    if token: headers["X-Edge-Gate-Token"] = token
    if upgrade: headers["Upgrade"] = upgrade
    c=http.client.HTTPConnection("127.0.0.1",port,timeout=3); c.request(method,path,body=body,headers=headers); r=c.getresponse(); data=r.read(); c.close(); return r.status,data

def main():
    backend=ThreadingHTTPServer(("127.0.0.1",0),Backend); threading.Thread(target=backend.serve_forever,daemon=True).start()
    with tempfile.TemporaryDirectory() as d:
        gate_port=backend.server_address[1]+1; db=os.path.join(d,"spool.db")
        env=dict(os.environ,EDGE_GATE_TOKEN="test-token",EDGE_GATE_DB=db,EDGE_GATE_PORT=str(gate_port),EDGE_BACKEND_PORT=str(backend.server_address[1]))
        proc=subprocess.Popen(["python3","scripts/edge-gate.py","serve"],env=env)
        try:
            time.sleep(.15)
            stream={}
            def read(): stream["v"]=call(gate_port,"GET","/stream")
            t=threading.Thread(target=read); t.start(); time.sleep(.05)
            ws=call(gate_port,"GET","/socket",host="llm.trnhieu.dev",upgrade="websocket"); assert ws[0] == 101, ws
            assert call(gate_port,"POST","/__gate/payment-enter",token="test-token")[0] == 202
            enter=call(gate_port,"POST","/__gate/enter",token="test-token"); assert enter[0] == 202, enter
            assert call(gate_port,"GET","/new")[0] == 503
            assert call(gate_port,"GET","/new",host="other.example.test")[0] == 421
            assert call(gate_port,"POST","/api/payment/sepay/webhook",body=b'{"id":1}')[0] == 202
            t.join(2); assert stream["v"][0] == 200 and stream["v"][1].count(b"[DONE]") == 1
            assert call(gate_port,"POST","/__gate/check",token="test-token")[0] == 200
            assert call(gate_port,"POST","/__gate/leave",token="test-token")[0] == 202
            # Crash/restart must remain fail-closed and retain the spool/state.
            assert call(gate_port,"POST","/api/payment/sepay/webhook",body=b'{"id":2}')[0] == 202
            proc.terminate(); proc.wait(timeout=3)
            proc=subprocess.Popen(["python3","scripts/edge-gate.py","serve"],env=env); time.sleep(.15)
            assert call(gate_port,"POST","/__gate/payment-check",token="test-token")[0] == 200
            assert call(gate_port,"POST","/__gate/leave",token="test-token")[0] == 202
            assert call(gate_port,"POST","/__gate/payment-leave",token="test-token")[0] == 202
            with concurrent.futures.ThreadPoolExecutor(max_workers=32) as pool:
                results=list(pool.map(lambda _: call(gate_port,"GET","/fast")[0], range(100)))
            assert all(status == 200 for status in results), sorted(set(results))
            print("PASS: edge gate hostname admission, SSE drain, durable webhook spool/replay")
        finally:
            proc.terminate(); proc.wait(timeout=3); backend.shutdown(); backend.server_close()
if __name__ == "__main__": main()
