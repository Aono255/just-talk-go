package config

// ASRResource identifies both the model generation and its billing product.
type ASRResource struct {
	ID, Label string
}

const DefaultASRResourceID = "volc.bigasr.sauc.duration"

func ASRResources() []ASRResource {
	return []ASRResource{
		{DefaultASRResourceID, "豆包 1.0（小时版）"},
		{"volc.seedasr.sauc.duration", "豆包 2.0（小时版）"},
		{"volc.bigasr.sauc.concurrent", "豆包 1.0（并发版）"},
		{"volc.seedasr.sauc.concurrent", "豆包 2.0（并发版）"},
	}
}
