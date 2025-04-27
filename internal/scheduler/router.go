package scheduler

import "context"

func NewRouter() RouterInterface {
	return &Router{}
}

func (r *Router) SendRequest(ctx context.Context, url string, headers map[string]string, payload []byte) ([]byte, int, error) {
	return nil, 0, nil
}
