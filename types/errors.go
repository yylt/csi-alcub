package types

type NeedUpdateErr []byte

func NewNeedUpdateError(s string) NeedUpdateErr {
	return NeedUpdateErr([]byte(s))
}

func (nue NeedUpdateErr) Error() string {
	return string(nue)
}

type AlreadyExist []byte

func NewAlreadyExistError(s string) AlreadyExist {
	return AlreadyExist([]byte(s))
}

func (nue AlreadyExist) Error() string {
	return string(nue)
}
