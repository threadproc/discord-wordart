build:
	go build -o discord-wordart main.go

deploy: build
	ssh root@bots.threadproc.io "systemctl stop discord-wordart"
	rsync -arv web discord-wordart config.conf root@bots.threadproc.io:/opt/discord-wordart/
	ssh root@bots.threadproc.io "systemctl start discord-wordart"