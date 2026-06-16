# protocms

ProtoCMS is a Prototype of a headless content management system.
It is a work in progress.

Usage examples:

```shell
# default dataset → data/default.json
go run . 

# named dataset → data/blog.json
go run . -dataset blog

# switch datasets without losing either
go run . -dataset staging
```

Test it out!

```shell
curl -X GET http://localhost:8080/api/content-types
curl http://localhost:8080/api/content-types | jq

curl -X GET http://localhost:8080/api/content/post \
    -H "Content-Type: application/json"

# List content (GET /api/content/{contentType})

curl http://localhost:8080/api/content/category | jq
curl http://localhost:8080/api/content/dish | jq
curl http://localhost:8080/api/content/drink | jq

# All starters (category_id=1)
curl http://localhost:8080/api/content/dish?category_id=1 | jq

# All pasta dishes (category_id=3)
curl "http://localhost:8080/api/content/dish?category_id=3" | jq

curl http://localhost:8080/api/content/drink | jq


curl http://localhost:8080/api/content/dish | jq

curl -X POST http://localhost:8080/api/content-types \
    -H "Content-Type: application/json"
    
curl -X POST http://localhost:8080/api/content/post \
    -H "Content-Type: application/json" \
    -d '{"title":"Hello","body":"World"}' 
    
curl -X POST http://localhost:8080/api/content/post \
    -H "Content-Type: application/json" \
    -d '{"title":"Post 2","body":"Post 2 body !"}'
    
curl -X GET http://localhost:8080/api/content/post    

curl -X GET http://localhost:8080/api/content/post/1

curl http://localhost:8080/api/stats

curl http://localhost:8080/api/stats | jq
curl http://localhost:8080/api/stats | jq '.dataset'
curl http://localhost:8080/api/stats | jq '.items_per_type'

curl http://localhost:8080/api/stats | python3 -m json.tool
curl http://localhost:8080/api/stats | fx
curl http://localhost:8080/api/stats | gron

# set a shell alias
alias jcurl='curl -s "$@" | jq'
jcurl http://localhost:8080/api/stats


```


### Samples with FieldTypes

```shell
curl -X POST http://localhost:8080/api/content-types \
  -H "Content-Type: application/json" \
  -d '{
    "name": "post",
    "fields": {
      "title": {"type": "text", "required": true},
      "body": {"type": "richText"},
      "published": {"type": "boolean", "default": false},
      "author": {"type": "reference", "refType": "author"},
      "tags": {"type": "select", "options": ["tech", "news", "tutorial"]}
    }
  }'
```