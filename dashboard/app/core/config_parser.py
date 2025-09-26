import re

CONFIG_PATH = "/Users/dharminpatel/ubuntu-auto-update/config.conf"

def parse_config():
    config = {}
    with open(CONFIG_PATH, "r") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            key, value = line.split("=", 1)
            config[key] = value.strip("\"")
    return config

def update_config(new_config):
    with open(CONFIG_PATH, "r") as f:
        lines = f.readlines()

    with open(CONFIG_PATH, "w") as f:
        for line in lines:
            if not line.strip() or line.strip().startswith("#"):
                f.write(line)
                continue
            key = line.split("=", 1)[0]
            if key in new_config:
                f.write(f"{key}={new_config[key]}\n")
            else:
                f.write(line)
