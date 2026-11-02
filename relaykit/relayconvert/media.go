package relayconvert

import relaymedia "github.com/TranNgocPhuongHieu/LLM-gateway/relaykit/relayconvert/internal/media"

type MediaResolver = relaymedia.MediaResolver

func SetMediaResolver(resolver MediaResolver) {
	relaymedia.SetMediaResolver(resolver)
}
