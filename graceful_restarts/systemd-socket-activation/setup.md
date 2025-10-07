# steps
* sudo cp /home/ankitkul/OneDrive/projects/gotry/graceful_restarts/systemd-socket-activation/sysdsockack.socket /etc/systemd/system/
* sudo cp /home/ankitkul/OneDrive/projects/gotry/graceful_restarts/systemd-socket-activation/sysdsockack.service /etc/systemd/system/

* sudo systemctl daemon-reload
* sudo systemctl enable sysdsockack.socket
* sudo systemctl enable sysdsockack.service

* sudo systemctl start sysdsockack.socket
* sudo systemctl start sysdsockack.service

* sudo systemctl restart sysdsockack.service

* sudo systemctl status sysdsockack.socket
* sudo systemctl status sysdsockack.service

# view logs 
* sudo journalctl -u sysdsockack.service -f
