.PHONY:
	local
	run
# The version that will be used in docker tags
VERSION ?= $(shell git rev-parse --short HEAD)


image: # Build docker image
	docker build -t kube-ephemeral-storage-exporter:$(VERSION) .

local: # Run go application locally
	go run main.go

run: image # Run docker container in foreground
	docker run -p 9000:9000 kube-ephemeral-storage-exporter:$(VERSION)
