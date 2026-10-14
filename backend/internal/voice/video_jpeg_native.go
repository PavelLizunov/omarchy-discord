package voice

/*
#cgo pkg-config: libturbojpeg
#include <turbojpeg.h>
#include <stdlib.h>
#include <limits.h>

static int resize_jpeg(const unsigned char *input, unsigned long size, int cap,
                      unsigned char **output, unsigned long *output_size) {
  tjhandle decoder = tjInitDecompress(), encoder = NULL;
  unsigned char *decoded = NULL, *scaled = NULL;
  int result = -1, w, h, subsampling, colorspace;
  if (!decoder || cap <= 0 || tjDecompressHeader3(decoder, input, size, &w, &h,
                                                &subsampling, &colorspace)) goto done;
  if (w <= 0 || h <= 0 || (long long)w*h > 8294400) goto done;
  tj3Set(decoder, TJPARAM_STOPONWARNING, 1);
  tj3Set(decoder, TJPARAM_MAXMEMORY, 64);
  if (h <= cap) { result = 0; goto done; }
  int target_w = (int)((long long)w*cap/h);
  if (target_w <= 0) target_w = 1;
  int count = 0;
  tjscalingfactor *factors = tjGetScalingFactors(&count);
  int sw = w, sh = h;
  for (int i = 0; i < count; i++) {
    int fw = TJSCALED(w, factors[i]), fh = TJSCALED(h, factors[i]);
    if (fh >= cap && fw >= target_w && fh < sh) { sw = fw; sh = fh; }
  }
  decoded = malloc((size_t)sw*sh*3);
  scaled = malloc((size_t)target_w*cap*3);
  if (!decoded || !scaled) goto done;
  if (tjDecompress2(decoder, input, size, decoded, sw, sw*3, sh, TJPF_RGB, 0)) goto done;
  for (int y = 0; y < cap; y++) {
    int sy = (int)((long long)y*sh/cap);
    for (int x = 0; x < target_w; x++) {
      int sx = (int)((long long)x*sw/target_w);
      unsigned char *dst = scaled + ((size_t)y*target_w+x)*3;
      const unsigned char *src = decoded + ((size_t)sy*sw+sx)*3;
      dst[0] = src[0]; dst[1] = src[1]; dst[2] = src[2];
    }
  }
  encoder = tjInitCompress();
  if (!encoder || tjCompress2(encoder, scaled, target_w, target_w*3, cap, TJPF_RGB,
                              output, output_size, TJSAMP_420, 92, 0)) goto done;
  if (*output_size > (4 << 20)) goto done;
  result = 1;
done:
  if (result != 1 && *output) { tjFree(*output); *output = NULL; }
  free(decoded); free(scaled);
  if (decoder) tjDestroy(decoder);
  if (encoder) tjDestroy(encoder);
  return result;
}
*/
import "C"

import "unsafe"

func resizeVideoJPEGNative(input []byte, height int) ([]byte, bool) {
	if len(input) == 0 || len(input) > cameraFrameLimit || !validQuality(height) {
		return nil, false
	}
	var output *C.uchar
	var size C.ulong
	status := C.resize_jpeg((*C.uchar)(unsafe.Pointer(&input[0])), C.ulong(len(input)), C.int(height), &output, &size)
	if status == 0 {
		return input, true
	}
	if status != 1 {
		return nil, false
	}
	defer C.tjFree(output)
	return C.GoBytes(unsafe.Pointer(output), C.int(size)), true
}
