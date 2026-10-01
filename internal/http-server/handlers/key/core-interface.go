package key

type Core interface {
	GenerateApiKey(username, scope string) (string, error)
}
