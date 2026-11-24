"use client";

import { Pause, Play } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";

interface ShowcaseSlide {
  src: string;
  alt: string;
  category: string;
  subtitle: string;
  href: string;
}

const SLIDES: readonly ShowcaseSlide[] = [
  {
    src: "/editorial/hero-espacios.jpg",
    alt: "Oficina y estudio flexible luminoso con mobiliario de madera e iluminación natural",
    category: "Oficina",
    subtitle: "Espacios de trabajo por hora, día o mes",
    href: "/categorias/oficina",
  },
  {
    src: "/editorial/hero-casa.jpg",
    alt: "Casa residencial luminosa con sala de estar y terraza al jardín",
    category: "Arriendos residenciales",
    subtitle: "Casas, departamentos, cabañas y habitaciones",
    href: "/categorias/inmuebles",
  },
  {
    src: "/editorial/hero-sala.jpg",
    alt: "Sala multipropósito equipada para kinesiología, talleres y reuniones",
    category: "Sala multipropósito",
    subtitle: "Talleres, consultas, clases y reuniones",
    href: "/categorias/sala_multiproposito",
  },
  {
    src: "/editorial/hero-quincho.jpg",
    alt: "Quincho moderno con pérgola de madera, parrilla y patio para celebraciones",
    category: "Quincho y eventos",
    subtitle: "Quinchos, terrazas y parcelas para celebrar",
    href: "/categorias/quincho",
  },
  {
    src: "/editorial/hero-local-bodega.jpg",
    alt: "Local flexible y bodega moderna ordenada con iluminación cálida",
    category: "Local flexible y bodega",
    subtitle: "Exhibición comercial, stands y almacenamiento",
    href: "/categorias/local_flexible",
  },
];

export function HeroShowcase() {
  const [activeIndex, setActiveIndex] = useState(0);
  const [paused, setPaused] = useState(false);

  useEffect(() => {
    if (paused) return;
    const mediaQuery =
      typeof window !== "undefined"
        ? window.matchMedia("(prefers-reduced-motion: reduce)")
        : null;
    if (mediaQuery?.matches) return;

    const timer = window.setInterval(() => {
      setActiveIndex((prev) => (prev + 1) % SLIDES.length);
    }, 4200);
    return () => window.clearInterval(timer);
  }, [paused]);

  const current = SLIDES[activeIndex];

  return (
    <div
      className="relative aspect-[16/10] w-full overflow-hidden rounded-2xl border border-border bg-background shadow-sm lg:aspect-[4/3]"
      aria-roledescription="carousel"
      aria-label="Categorías de espacios disponibles en EspaciGo"
    >
      {SLIDES.map((slide, index) => (
        <div
          key={slide.src}
          aria-hidden={index !== activeIndex}
          className={`absolute inset-0 transition-opacity duration-700 ease-in-out ${
            index === activeIndex
              ? "opacity-100"
              : "pointer-events-none opacity-0"
          }`}
        >
          <Image
            src={slide.src}
            alt={slide.alt}
            fill
            priority={index === 0}
            sizes="(max-width: 1024px) 100vw, 44vw"
            className="object-cover"
          />
        </div>
      ))}

      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-28 bg-gradient-to-t from-black/70 via-black/30 to-transparent" />

      <div className="absolute inset-x-3 bottom-3 flex items-end justify-between gap-3 sm:inset-x-4 sm:bottom-4">
        <Link
          href={current.href}
          className="group min-w-0 rounded-xl bg-background/95 px-3.5 py-2 shadow-sm backdrop-blur-sm transition-colors hover:bg-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="inline-flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--brand-primary-soft-ink)]">
            <span
              aria-hidden="true"
              className="size-1.5 rounded-full bg-[var(--brand-accent)]"
            />
            {current.category}
          </span>
          <p className="mt-0.5 truncate text-xs font-medium text-foreground sm:text-sm">
            {current.subtitle}
          </p>
        </Link>

        <div className="flex shrink-0 items-center gap-1.5 rounded-full bg-background/90 px-2 py-1.5 shadow-sm backdrop-blur-sm">
          <div className="flex items-center gap-1" role="tablist" aria-label="Elegir categoría">
            {SLIDES.map((slide, index) => (
              <button
                key={slide.category}
                type="button"
                role="tab"
                aria-selected={index === activeIndex}
                aria-label={`Ver ${slide.category}`}
                onClick={() => setActiveIndex(index)}
                className={`h-2 rounded-full transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                  index === activeIndex
                    ? "w-5 bg-primary"
                    : "w-2 bg-[var(--brand-control-border)] hover:bg-foreground/60"
                }`}
              />
            ))}
          </div>
          <button
            type="button"
            onClick={() => setPaused((prev) => !prev)}
            aria-label={
              paused ? "Reanudar presentación de espacios" : "Pausar presentación de espacios"
            }
            className="ml-1 inline-flex size-7 items-center justify-center rounded-full text-foreground transition-colors hover:bg-[var(--brand-primary-soft)] hover:text-[var(--brand-primary-soft-ink)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {paused ? (
              <Play className="size-3.5" aria-hidden="true" />
            ) : (
              <Pause className="size-3.5" aria-hidden="true" />
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
