package utils

func ConvertToUint16(ids []uint32) []uint16 {
	result := make([]uint16, len(ids))
	for i, id := range ids {
		result[i] = uint16(id)
	}
	return result
}

func ConvertToUint32(ids []uint16) []uint32 {
	result := make([]uint32, len(ids))
	for i, id := range ids {
		result[i] = uint32(id)
	}
	return result
}
