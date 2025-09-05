package qr_stat

type Core interface {
	FollowQr(smartSenderId string) error
	GetQrStat(group, phone string) error
}
