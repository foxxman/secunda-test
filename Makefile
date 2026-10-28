.PHONY: run deploy test down

run:
	docker compose up --build

deploy:
	docker compose up -d --build

test:
	go test ./... -count=1 -cover

down:
	docker compose down
