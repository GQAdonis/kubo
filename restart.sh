#!/bin/bash

docker compose down && COMPOSE_BAKE=true docker compose up -d --build

