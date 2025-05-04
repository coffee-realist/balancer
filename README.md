Go HTTP Балансировщик Нагрузки
==============================

Этот проект реализует HTTP reverse-proxy балансировщик нагрузки на Go с несколькими алгоритмами (round-robin, least-connections, power-of-two-choices, adaptive), health-check’ами, плавным завершением работы и rate-limiter’ом на основе token-bucket с per-client CRUD API на SQLite.

Необходимые зависимости
-----------------------

*   Go 1.24+
*   Docker & Docker Compose

1\. Клонирование и запуск тестов
--------------------------------

    # Клонируем репозиторий
    git clone https://github.com/coffee-realist/balancer.git
    cd balancer
    
    # Запускаем все Go-тесты с детектором гонок
    go test ./... -v -race


2\. Интеграционные и бенчмарк-тесты
-----------------------------------


    # Интеграционные тесты
    go test ./integration -v -timeout 2m
    
    # Бенчмарки
    go test ./... -bench=. -timeout 5m


3\. Запуск через Docker Compose
-------------------------------


    # Переходим из корня проекта в папку с docker-compose.yaml
    cd deployments
    # Поднимаем контейнеры бэкендов и балансировщика
    docker-compose up



4\. Регистрация клиента для rate-limit
--------------------------------------

Создаём API-ключ с лимитом:


    docker run --rm --network deployment_lb-net curlimages/curl \
      -X POST "http://balancer:8080/clients?id=bench-client" \
      -H "Content-Type: application/json" \
      -d '{"capacity":1000,"refill_rate":1000,"refill_interval":1}'


5\. Нагрузочное тестирование ApacheBench (в Docker)
---------------------------------------------------

Запускаем 10000 запросов с параллелизмом 100, передавая заголовок X-API-Key:


    docker run --rm --network deployment_lb-net jordi/ab \
      -n 10000 -c 100 \
      -H "X-API-Key: bench-client" \
      http://balancer:8080/foo


6\. Очистка
-----------


    docker-compose down


* * *

Подробные параметры (алгоритм балансировки, интервалы health-check, настройки rate-limiter’а) задаются в `config.yaml`.
