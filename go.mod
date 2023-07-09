module github.com/AlteredLabsAI/jonah-sales-verification-lambda

go 1.18

require github.com/AlteredLabsAI/jonah-shared v0.0.0-00010101000000-000000000000

require (
	github.com/Integralist/go-elasticache v0.0.0-20190122104721-fb0aee05cd4e // indirect
	github.com/aws/aws-lambda-go v1.36.0 // indirect
	github.com/aws/aws-sdk-go v1.44.125 // indirect
	github.com/bradfitz/gomemcache v0.0.0-20221031212613-62deef7fc822 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/integralist/go-findroot v0.0.0-20160518114804-ac90681525dc // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/lib/pq v1.10.7 // indirect
)

replace github.com/AlteredLabsAI/jonah-shared => ../jonah-shared
