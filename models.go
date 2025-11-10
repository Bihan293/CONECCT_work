package main

type Profile struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	Description string `json:"description"`
	PhotoFileID string `json:"photo_file_id"`
}

type Order struct {
	ID          int64  `json:"id"`
	CreatorID   int64  `json:"creator_id"`
	Category    string `json:"category"`
	Text        string `json:"text"`
	PhotoFileID string `json:"photo_file_id"`
	Complaints  int    `json:"complaints"`
}
