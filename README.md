# countdown-as-a-service

countdown as a service

App listens on port 8080

# URL's

- /
- /start
- /status

# API URL's

We have two API URL's 

- `/api/status/`
- `/api/status/{id}`

You may query the API using CURL and JQ

```
curl -s http://localhost:8080/api/status/ | jq .[]
```

Select only ID 2

```
curl -s http://localhost:8080/api/status/ | jq '.[] | select(.id == 2)'
```
