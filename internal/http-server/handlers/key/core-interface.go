package key

type Core interface {
	GenerateApiKey(username string) (string, error)
}
