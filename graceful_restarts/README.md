# What is the primary goal of graceful restart ?
    - Do not give connection error for new connections
    - Keep serving the old connections 
    - Drain properly 
    - Handle any new intialization errors and backoff

# Couple of ways to do it 
* Exec the same process once again (in place replacement)
* fork and exec same process 
  * can't use on same port 2 process, 
  * may be can with SO_REUSEPORT option
  * race conditions 
* Nginx type (master + worker),blue green type
* Standard FD based handoff 
* Systemd socket activation 


# 📝 Summary of the Implementation Plan

We’re going to write a **single Go program** (no external libraries) that demonstrates **zero-downtime graceful restart**.  
It will:

1. **Listen on `:8080`** and respond `hello world + PID` so we can see which process serves which request.
2. Implement the **classic graceful restart pattern**:
   - Parent process creates a listening socket.
   - On `SIGHUP` it `exec`s a **new binary** (new version) and passes the listener FD to it.
   - Parent stops accepting new connections but finishes existing ones.
   - Child immediately starts accepting on the same socket → zero downtime.
3. **Pass file descriptors** via `cmd.ExtraFiles` and environment variables:
   - Listener FD → child inherits as FD=3.
   - “I’m ready” pipe write end → child inherits as FD=4.
4. **Handshake**: child writes “ready” to the pipe when it’s actually ready (e.g. after first transaction). Parent only closes its listener after receiving that signal.
5. **Slow request simulation**: every Nth request takes 10 seconds with per-second heartbeats printed to stdout, so you can actually see the old PID finishing a long request while the new PID serves new requests.
6. **No fatal exit on failure**: if new binary fails or never signals ready, the old server keeps accepting connections and logs a warning.

This is essentially a mini version of what **nginx**, **haproxy**, **envoy**, etc. do when they reload workers.

---

` # 📚 **Glossary & Key Terms** `

| Term                          | Meaning                                                                                                                                                                                                            |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **FD (File Descriptor)**      | An integer handle the kernel gives you for an open file/socket.                                                                                                                                                    |
| **Listener FD**               | The file descriptor for the TCP socket listening on `:8080`.                                                                                                                                                       |
| **`cmd.ExtraFiles`**          | Go’s way to hand additional FDs to a child process at `exec` time. First one appears as FD=3.                                                                                                                      |
| **dup’d `*os.File`**          | A new `*os.File` created from the same underlying FD using `dup()` so you can safely pass it to a child.                                                                                                           |
| **`SIGHUP`**                  | UNIX signal used here to trigger a graceful upgrade.                                                                                                                                                               |
| **`SIGTERM`/`SIGINT`**        | Signals to trigger a graceful shutdown (no new child, just drain and exit).                                                                                                                                        |
| **Draining**                  | Stop accepting new connections but continue serving existing ones until complete.                                                                                                                                  |
| **Transaction vs Connection** | Connection = one TCP session. Transaction = one logical request/response cycle on that connection (SMTP, Milter, Redis). For graceful restart you must decide whether to drain at connection or transaction level. |
| **`syscall.RawConn`**         | Lets you get at the raw FD to call low-level socket options (we’ll show it for demonstration).                                                                                                                     |
| **Inherited pipe**            | A simple `os.Pipe()` you give to the child so it can send a “I’m ready” signal back to the parent.                                                                                                                 |

---

` # 🤔 **Discussion / Curiosity Prompts** `

Use these as “did you think about…” questions to engage your team:



### table 
| Feature                        | **Systemd Socket Activation**              | **Tableflip / Manual FD Handoff**                  |
| ------------------------------ | ------------------------------------------ | -------------------------------------------------- |
| Who owns the socket            | systemd (PID 1)                            | your process                                       |
| What happens on restart        | systemd passes FD to new process           | parent forks new process itself                    |
| Old connections during restart | dropped unless drained externally          | parent continues serving them                      |
| Coordination mechanism         | none                                       | explicit “ready” pipe                              |
| Best suited for                | simple daemons (e.g. sshd, dbus, journald) | high-traffic servers (e.g. nginx, Cloudflare edge) |
| Downtime on port bind          | none                                       | none                                               |
| Downtime on in-flight requests | yes (if restart)                           | none                                               |
| Who controls upgrade timing    | systemd                                    | your process logic                                 |
| Complexity                     | very simple                                | higher, but total control                          |


REferences: 
* https://blog.cloudflare.com/graceful-upgrades-in-go/
* https://blog.cloudflare.com/20-percent-internet-upgrade/
* https://blog.cloudflare.com/oxy-the-journey-of-graceful-restarts/




#### systemd socket activation 


```bash
<!-- /etc/systemd/system/myapp.socket -->
[Unit]
Description=MyApp incoming connections

[Socket]
ListenStream=0.0.0.0:8080     # app traffic
ListenStream=127.0.0.1:9090   # prometheus metrics
ListenDatagram=514            # syslog datagrams, for example

# for unix domain: ListenStream=/run/myapp.sock
# Accept=no means a single socket for all connections

[Install]
WantedBy=sockets.target
```

```bash
/etc/systemd/system/myapp.service
[Unit]
Description=MyApp service
Requires=myapp-main.socket <myapp-metrics.socket optional multiple socket files>
After=network.target

[Service]
ExecStart=/usr/local/bin/myapp
# systemd hands over sockets via $LISTEN_FDS/$LISTEN_PID
Restart=always
NonBlocking=true

[Install]
WantedBy=multi-user.target
```

Either way, systemd sets the environment:

LISTEN_PID = your process PID
LISTEN_FDS = number of descriptors passed


The FDs start at 3 and go up sequentially.
