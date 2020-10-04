export COMPOSE_FILE = docker/dev.yml

dev:
	@docker-compose rm -f 
	@docker-compose up --build
live:
	@docker run strife-app
