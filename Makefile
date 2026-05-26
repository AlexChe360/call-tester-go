BINARY = call-tester
RPI_HOST = pi@192.168.0.108
RPI_DIR = /home/pi/call-tester

build:
	go build -o $(BINARY) ./cmd/call-tester

build-rpi:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BINARY)-linux-arm64 ./cmd/call-tester

deploy: build-rpi
	ssh $(RPI_HOST) "mkdir -p $(RPI_DIR)/scenarios $(RPI_DIR)/reports"
	scp $(BINARY)-linux-arm64 $(RPI_HOST):$(RPI_DIR)/$(BINARY)
	scp config.yaml $(RPI_HOST):$(RPI_DIR)/
	scp scenarios/*.yaml $(RPI_HOST):$(RPI_DIR)/scenarios/
	scp setup_rpi.sh $(RPI_HOST):$(RPI_DIR)/
	ssh $(RPI_HOST) "chmod +x $(RPI_DIR)/$(BINARY) $(RPI_DIR)/setup_rpi.sh"
	@echo ""
	@echo "Done. On RPi:"
	@echo "  cd $(RPI_DIR) && ./call-tester check"

check:
	./$(BINARY) check

run:
	./$(BINARY) run scenarios/full_test.yaml

docker-build:
	docker buildx build --platform linux/arm64 -t call-tester:latest --load .

docker-deploy: docker-build
	docker save call-tester:latest | ssh $(RPI_HOST) "docker load"

clean:
	rm -f $(BINARY) $(BINARY)-linux-arm64

.PHONY: build build-rpi deploy check run docker-build docker-deploy clean
