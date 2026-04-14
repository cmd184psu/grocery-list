BINARY      := grocery-list
BUILD_DIR   := ./bin
CMD         := ./cmd/server
INSTALL_DIR := /opt/grocery-list
CONFIG_PATH := ~/.grocery.json

.PHONY: all build run run-tls clean init-config install uninstall

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD)

run: build
	$(BUILD_DIR)/$(BINARY) --web-dir ./web --data-file ./items.json

run-tls: build
	$(BUILD_DIR)/$(BINARY) --config $(CONFIG_PATH)

clean:
	rm -rf $(BUILD_DIR)

init-config: build
	$(BUILD_DIR)/$(BINARY) --init-config --config $(CONFIG_PATH)

# Installs binary + web assets + systemd unit.
# Assumes /opt/grocery-list already exists (or is a symlink).
install: build
	@echo "Installing to $(INSTALL_DIR)..."
	sudo mkdir -p $(INSTALL_DIR)/web
	sudo cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/
	sudo cp -r web/* $(INSTALL_DIR)/web/
	sudo cp grocery-list.service /etc/systemd/system/
	sudo systemctl daemon-reload
	@echo ""
	@echo "Done. To enable and start:"
	@echo "  sudo systemctl enable --now grocery-list"

uninstall:
	sudo systemctl stop grocery-list    || true
	sudo systemctl disable grocery-list || true
	sudo rm -f /etc/systemd/system/grocery-list.service
	sudo systemctl daemon-reload
	@echo "Binary and service removed. $(INSTALL_DIR) left intact."
