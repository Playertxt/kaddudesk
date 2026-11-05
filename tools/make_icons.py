"""Generates all KadduDesk icons (server tray, Windows client, Android, web) from code."""
import os
from PIL import Image, ImageDraw, ImageFilter

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
S = 1024  # master size


def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(len(a)))


def master():
    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    # diagonal gradient pumpkin orange -> deep coral
    grad = Image.new("RGBA", (S, S))
    gp = grad.load()
    c1, c2 = (255, 158, 44, 255), (238, 66, 70, 255)
    for y in range(S):
        for x in range(0, S, 4):
            c = lerp(c1, c2, (x + y) / (2 * S))
            for k in range(4):
                gp[x + k, y] = c
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([40, 40, S - 40, S - 40], radius=230, fill=255)
    img.paste(grad, (0, 0), mask)

    d = ImageDraw.Draw(img)
    white = (255, 255, 255, 255)
    # monitor
    d.rounded_rectangle([200, 250, 824, 650], radius=56, outline=white, width=58)
    d.rounded_rectangle([430, 650, 594, 740], radius=10, fill=white)
    d.rounded_rectangle([330, 730, 694, 790], radius=30, fill=white)
    # lightning bolt (speed)
    bolt = [(560, 300), (420, 480), (505, 480), (455, 610), (610, 420), (520, 420), (575, 300)]
    d.polygon(bolt, fill=white)
    return img


def save_ico(img, path, sizes):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    img.save(path, format="ICO", sizes=[(s, s) for s in sizes])


def main():
    m = master()
    save_ico(m, os.path.join(ROOT, "server", "assets", "icon.ico"), [16, 20, 24, 32, 48, 64, 128, 256])
    save_ico(m, os.path.join(ROOT, "build", "icon.ico"), [16, 24, 32, 48, 64, 128, 256])
    m.resize((512, 512), Image.LANCZOS).save(os.path.join(ROOT, "build", "icon.png"))
    m.resize((192, 192), Image.LANCZOS).save(os.path.join(ROOT, "server", "web", "icon.png"))
    os.makedirs(os.path.join(ROOT, "clients", "launcher"), exist_ok=True)
    m.resize((192, 192), Image.LANCZOS).save(os.path.join(ROOT, "clients", "launcher", "icon.png"))
    for name, px in {"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}.items():
        p = os.path.join(ROOT, "android", "res", "mipmap-" + name)
        os.makedirs(p, exist_ok=True)
        m.resize((px, px), Image.LANCZOS).save(os.path.join(p, "ic_launcher.png"))
    print("icons ok")


if __name__ == "__main__":
    main()
