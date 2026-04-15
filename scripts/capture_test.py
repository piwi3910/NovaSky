#!/usr/bin/env python3
"""Capture frames in RAW8, RAW16, RGB24 from camera via INDI for comparison.
Uses gain 0 and auto-detected exposure from last known good value."""
import socket, time, base64
import numpy as np
from PIL import Image
from astropy.io import fits
import cv2

def indi_send(sock, xml):
    sock.sendall(xml.encode())

def indi_capture(sock, device, format_name, exposure=0.04, gain=0):
    # Set gain to 0
    indi_send(sock, f'<newNumberVector device="{device}" name="CCD_CONTROLS"><oneNumber name="Gain">{gain}</oneNumber></newNumberVector>')
    time.sleep(0.3)

    # Set format
    indi_send(sock, f'<newSwitchVector device="{device}" name="CCD_CAPTURE_FORMAT">')
    for fmt in ['ASI_IMG_RAW8', 'ASI_IMG_RAW16', 'ASI_IMG_RGB24']:
        state = 'On' if fmt == format_name else 'Off'
        indi_send(sock, f'<oneSwitch name="{fmt}">{state}</oneSwitch>')
    indi_send(sock, '</newSwitchVector>')
    time.sleep(1)

    # Trigger capture
    indi_send(sock, f'<newNumberVector device="{device}" name="CCD_EXPOSURE"><oneNumber name="CCD_EXPOSURE_VALUE">{exposure}</oneNumber></newNumberVector>')

    # Wait for BLOB
    data = b''
    start = time.time()
    while time.time() - start < 30:
        try:
            chunk = sock.recv(1048576)
            if chunk:
                data += chunk
                if b'</setBLOBVector>' in data:
                    break
        except socket.timeout:
            continue

    idx = data.find(b'<oneBLOB')
    if idx < 0:
        return None
    start_data = data.index(b'>', idx) + 1
    end_data = data.index(b'</oneBLOB>', start_data)
    b64 = data[start_data:end_data].strip()
    return base64.b64decode(b64)

# Connect
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.settimeout(10)
sock.connect(('localhost', 7624))
indi_send(sock, '<getProperties version="1.7"/>')
indi_send(sock, '<enableBLOB device="">Also</enableBLOB>')
time.sleep(3)

device = 'ZWO CCD ASI676MC'

# Use short exposure for daytime (our auto-exposure settled around 0.04-0.05ms)
exposure = 0.00004  # 0.04ms in seconds

for fmt, label in [('ASI_IMG_RAW8', 'raw8'), ('ASI_IMG_RAW16', 'raw16'), ('ASI_IMG_RGB24', 'rgb24')]:
    print(f'\nCapturing {label} (gain=0, exp={exposure*1000:.3f}ms)...')
    blob = indi_capture(sock, device, fmt, exposure, gain=0)
    if blob is None:
        print(f'  FAILED')
        continue

    fpath = f'/tmp/test_{label}.fits'
    with open(fpath, 'wb') as f:
        f.write(blob)

    try:
        with fits.open(fpath) as hdul:
            d = hdul[0].data
            h = hdul[0].header
            d = np.squeeze(d)
            print(f'  BITPIX={h.get("BITPIX")} BZERO={h.get("BZERO","none")} shape={d.shape} dtype={d.dtype}')
            print(f'  min={np.min(d)} max={np.max(d)} median={np.median(d):.0f}')

            if label == 'rgb24':
                # RGB24: FITS stores as 3xHxW, transpose to HxWx3
                if d.ndim == 3 and d.shape[0] == 3:
                    d = np.transpose(d, (1, 2, 0))
                # Already 8-bit RGB from camera - save directly
                img = d.astype(np.uint8)
                Image.fromarray(img).save(f'/tmp/test_{label}.jpg', quality=90)
                print(f'  Direct RGB from camera - no debayer needed')
            elif label == 'raw16':
                # RAW16: debayer with OpenCV using indi-allsky mapping
                bayer_code = cv2.COLOR_BAYER_BG2BGR
                bgr = cv2.cvtColor(d, bayer_code)
                rgb = cv2.cvtColor(bgr, cv2.COLOR_BGR2RGB)
                img = np.right_shift(rgb, 8).astype(np.uint8)
                Image.fromarray(img).save(f'/tmp/test_{label}.jpg', quality=90)
                print(f'  Debayered with BayerBG, shifted >>8')
            elif label == 'raw8':
                # RAW8: debayer with OpenCV
                bayer_code = cv2.COLOR_BAYER_BG2BGR
                bgr = cv2.cvtColor(d, bayer_code)
                rgb = cv2.cvtColor(bgr, cv2.COLOR_BGR2RGB)
                img = rgb.astype(np.uint8)
                Image.fromarray(img).save(f'/tmp/test_{label}.jpg', quality=90)
                print(f'  Debayered with BayerBG')

            # Print sky region color
            sky = img[img.shape[0]//6:img.shape[0]//4, img.shape[1]//3:2*img.shape[1]//3]
            if sky.ndim == 3:
                print(f'  Sky RGB: [{sky[:,:,0].mean():.0f}, {sky[:,:,1].mean():.0f}, {sky[:,:,2].mean():.0f}]')

            print(f'  JPEG saved: /tmp/test_{label}.jpg')
    except Exception as e:
        print(f'  Error: {e}')
        import traceback
        traceback.print_exc()

# Restore RAW16
indi_send(sock, f'<newSwitchVector device="{device}" name="CCD_CAPTURE_FORMAT"><oneSwitch name="ASI_IMG_RAW16">On</oneSwitch></newSwitchVector>')
sock.close()
print('\nDone. Restored RAW16 mode.')
