"use client";

import { useEffect, useRef, useState } from "react";
import { PublicMediaImage } from "./public-media-image";

export type PublicGalleryImage = { src: string; fallbackSrc?: string };

export function PublicSpaceMarketplaceGallery({
  images,
  productName,
}: {
  images: readonly PublicGalleryImage[];
  productName: string;
}) {
  const [selected, setSelected] = useState<number | null>(null);
  const opener = useRef<HTMLButtonElement | null>(null);
  const dialog = useRef<HTMLDialogElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);
  const wasOpen = useRef(false);
  const count = images.length;

  useEffect(() => {
    const open = selected !== null;
    if (open && !wasOpen.current) {
      dialog.current?.showModal();
      closeRef.current?.focus();
      wasOpen.current = true;
    } else if (!open && wasOpen.current) {
      dialog.current?.close();
      wasOpen.current = false;
      opener.current?.focus();
    }
  }, [selected]);

  useEffect(() => {
    if (selected === null) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setSelected(null);
      } else if (event.key === "Tab") {
        const controls = dialog.current?.querySelectorAll<HTMLElement>(
          "button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex='-1'])",
        );
        if (!controls?.length) {
          event.preventDefault();
          dialog.current?.focus();
          return;
        }
        const first = controls[0];
        const last = controls[controls.length - 1];
        if (
          event.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === dialog.current ||
            !dialog.current?.contains(document.activeElement))
        ) {
          event.preventDefault();
          last.focus();
        } else if (
          !event.shiftKey &&
          (document.activeElement === last ||
            !dialog.current?.contains(document.activeElement))
        ) {
          event.preventDefault();
          first.focus();
        }
      } else if (event.key === "ArrowRight") {
        setSelected((index) => (index === null ? null : (index + 1) % count));
      } else if (event.key === "ArrowLeft") {
        setSelected((index) =>
          index === null ? null : (index - 1 + count) % count,
        );
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [selected, count]);

  const visible = images.map((image, index) => ({ image, index }));
  const open = (index: number, button: HTMLButtonElement) => {
    opener.current = button;
    setSelected(index);
  };
  const move = (delta: number) =>
    setSelected((index) =>
      index === null ? null : (index + delta + count) % count,
    );

  return (
    <section aria-label="Galería de imágenes" className="min-w-0 space-y-3">
      {count === 0 ? (
        <div className="grid aspect-[4/3] place-items-center rounded-xl border border-border bg-muted/30 p-6 text-center text-sm text-muted-foreground">
          Este espacio no tiene imágenes públicas.
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 grid-rows-2 gap-2 overflow-hidden rounded-xl">
            {visible.slice(0, 4).map(({ image, index }, cell) => (
              <button
                key={`${image.src}-${index}`}
                type="button"
                onClick={(event) => open(index, event.currentTarget)}
                aria-label={
                  cell === 3 && count > 4
                    ? `+${count - 4} fotos, ver imagen ${index + 1} de ${count}`
                    : `Ver imagen ${index + 1} de ${count}`
                }
                className="relative aspect-square min-h-28 overflow-hidden bg-muted/30 focus-visible:outline-2 focus-visible:outline-offset-2"
              >
                <PublicMediaImage
                  src={image.src}
                  fallbackSrc={image.fallbackSrc}
                  alt={`${productName}, imagen ${index + 1}`}
                  fill
                  unoptimized
                  sizes="(max-width: 768px) 100vw, 33vw"
                  className="object-cover"
                />
                {cell === 3 && count > 4 ? (
                  <span className="absolute inset-0 grid place-items-center bg-black/55 text-lg font-semibold text-white">
                    +{count - 4}
                  </span>
                ) : null}
              </button>
            ))}
          </div>
          <div className="flex items-center justify-between gap-3">
            <span aria-live="polite" className="text-sm text-muted-foreground">
              {selected !== null ? `${selected + 1}/${count}` : `1/${count}`}
            </span>
            {count > 1 ? (
              <div
                aria-label="Miniaturas"
                className="flex max-w-[70%] gap-2 overflow-x-auto"
              >
                {images.map((image, index) => (
                  <button
                    key={`${image.src}-${index}`}
                    type="button"
                    onClick={(event) => open(index, event.currentTarget)}
                    aria-label={`Abrir miniatura ${index + 1} de ${count}`}
                    className="relative h-12 w-12 shrink-0 overflow-hidden rounded-md border border-border focus-visible:outline-2"
                  >
                    <PublicMediaImage
                      src={image.src}
                      fallbackSrc={image.fallbackSrc}
                      alt=""
                      fill
                      unoptimized
                      sizes="48px"
                      className="object-cover"
                    />
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </>
      )}
      {selected !== null ? (
        <dialog
          ref={dialog}
          aria-label={`Imagen ${selected + 1} de ${count}: ${productName}`}
          className="m-auto w-[min(90vw,80rem)] max-h-[calc(100dvh-2rem)] overflow-auto bg-transparent p-0 text-white backdrop:bg-black/90"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setSelected(null);
          }}
        >
          <div className="relative flex min-w-0 max-h-full max-w-6xl flex-col items-center gap-3 outline-none">
            <button
              ref={closeRef}
              type="button"
              onClick={() => setSelected(null)}
              aria-label="Cerrar galería"
              className="absolute right-2 top-2 z-10 min-h-11 rounded bg-white px-3 text-black focus-visible:outline-2"
            >
              Cerrar
            </button>
            {count > 1 ? (
              <button
                type="button"
                onClick={() => move(-1)}
                aria-label="Imagen anterior"
                className="absolute left-2 top-1/2 z-10 min-h-11 rounded bg-white px-3 text-black"
              >
                Anterior
              </button>
            ) : null}
            <div className="relative aspect-[4/3] max-h-[72dvh] w-full overflow-visible bg-black/20">
              <PublicMediaImage
                key={selected}
                src={images[selected].src}
                fallbackSrc={images[selected].fallbackSrc}
                alt={`${productName}, imagen ${selected + 1}`}
                fill
                unoptimized
                sizes="(max-width: 768px) 90vw, 80rem"
                className="object-contain"
              />
            </div>
            {count > 1 ? (
              <button
                type="button"
                onClick={() => move(1)}
                aria-label="Imagen siguiente"
                className="absolute right-2 top-1/2 z-10 min-h-11 rounded bg-white px-3 text-black"
              >
                Siguiente
              </button>
            ) : null}
            <p className="rounded-full bg-black px-3 py-1 text-white">
              {selected + 1}/{count}
            </p>
          </div>
        </dialog>
      ) : null}
    </section>
  );
}
