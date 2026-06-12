package database

import "time"

type storageSession struct {
	AppName   string  `gorm:"primaryKey;column:app_name"`
	UserID    string  `gorm:"primaryKey;column:user_id"`
	ID        string  `gorm:"primaryKey;column:id"`
	State     jsonMap `gorm:"column:state;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time `gorm:"index"`

	Events []storageEvent `gorm:"foreignKey:AppName,UserID,SessionID;references:AppName,UserID,ID;constraint:OnDelete:CASCADE"`
}

func (storageSession) TableName() string { return "yu_sessions" }

type storageEvent struct {
	AppName   string `gorm:"primaryKey;column:app_name"`
	UserID    string `gorm:"primaryKey;column:user_id"`
	SessionID string `gorm:"primaryKey;column:session_id"`
	ID        string `gorm:"primaryKey;column:id"`

	Sequence     int64     `gorm:"column:sequence;not null;index:yu_session_events_order_idx,priority:4"`
	InvocationID string    `gorm:"column:invocation_id"`
	Type         string    `gorm:"column:type;not null"`
	Author       string    `gorm:"column:author"`
	Message      jsonValue `gorm:"column:message;not null"`
	Error        string    `gorm:"column:error"`
	Partial      bool      `gorm:"column:partial;not null;default:false"`
	CreatedAt    time.Time `gorm:"column:created_at;precision:6"`

	Session storageSession `gorm:"foreignKey:AppName,UserID,SessionID;references:AppName,UserID,ID"`
}

func (storageEvent) TableName() string { return "yu_session_events" }
