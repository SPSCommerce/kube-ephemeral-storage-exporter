.PHONY:
	local
	run
# The version that will be used in docker tags
VERSION ?= $(shell git rev-parse --short HEAD)


image: build_ci # Build executable and docker image
	docker build -t k8s-ephemeral-storage-exporter:$(VERSION) .

local: # Run go application locally
	go run main.go

run: image # Run docker container in foreground
	docker run -p 9000:9000 k8s-ephemeral-storage-exporter:$(VERSION)
