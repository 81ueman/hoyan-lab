package solver

func DefaultBackend() Backend {
	return Z3Backend{}
}
