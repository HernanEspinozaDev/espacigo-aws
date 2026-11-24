export interface CategoryGuideInfo {
  image: string;
  imageAlt: string;
  tagline: string;
  description: string;
  examples: string[];
  howToPublish: string;
}

const DEFAULT_GUIDE: CategoryGuideInfo = {
  image: "/editorial/hero-espacios.jpg",
  imageAlt: "Espacio flexible luminoso listo para arrendar",
  tagline: "Espacios flexibles por hora, día o mes",
  description:
    "Publica o arrienda espacios acondicionados con reglas claras, ubicación en el mapa y contratos digitales.",
  examples: ["Espacios equipados", "Tarifas por bloque", "Reserva directa"],
  howToPublish:
    "Inicia sesión, verifica tu identidad, marca la ubicación en el mapa, sube fotos reales, define tus tarifas y publica gratis.",
};

export const CATEGORY_GUIDES: Record<string, CategoryGuideInfo> = {
  oficina: {
    image: "/editorial/hero-espacios.jpg",
    imageAlt: "Oficina moderna luminosa con escritorios y luz natural",
    tagline: "Trabaja con privacidad o en equipo",
    description:
      "Oficinas privadas, estaciones de cowork y box profesionales listos para trabajar por hora, día o mes.",
    examples: ["Oficina privada", "Box profesional", "Puesto de trabajo"],
    howToPublish:
      "Indica capacidad máxima, metros cuadrados, conectividad y horarios disponibles para recibir reservas por hora, día o mes.",
  },
  sala_multiproposito: {
    image: "/editorial/hero-sala.jpg",
    imageAlt:
      "Sala multipropósito luminosa para talleres, terapias o reuniones",
    tagline: "Reuniones, talleres, clases y consultas",
    description:
      "Salas versátiles equipadas para sesiones de kinesiología, capacitaciones, directorio o actividades grupales.",
    examples: ["Sala de reuniones", "Box terapéutico", "Sala de talleres"],
    howToPublish:
      "Detalla el equipamiento incluido, capacidad de asistentes y bloques horarios para que los usuarios reserven directo.",
  },
  bodega: {
    image: "/editorial/cat-bodega.jpg",
    imageAlt: "Bodega y espacio de almacenamiento limpio y organizado",
    tagline: "Almacenamiento seguro y accesible",
    description:
      "Bodegas y espacios de guardado para inventario, mudanzas, equipamiento o mercadería con acceso controlado.",
    examples: ["Minibodega", "Bodega comercial", "Depósito de inventario"],
    howToPublish:
      "Especifica dimensiones en m², condiciones de acceso, seguridad del recinto y tarifa diaria o mensual.",
  },
  estacionamiento: {
    image: "/editorial/cat-estacionamiento.jpg",
    imageAlt: "Espacio de estacionamiento amplio y señalizado",
    tagline: "Estacionamientos por hora, día o mes",
    description:
      "Estacionamientos privados en edificios, casas o recintos comerciales cerca de zonas de alta demanda.",
    examples: [
      "Estacionamiento cubierto",
      "Estacionamiento residencial",
      "Uso diario",
    ],
    howToPublish:
      "Ubica el pin en el mapa, indica si es techado, altura máxima o tipo de acceso y publica tu tarifa gratis.",
  },
  local_flexible: {
    image: "/editorial/hero-local-bodega.jpg",
    imageAlt: "Local comercial flexible con iluminación cálida y vitrina",
    tagline: "Exhibición, pop-up stores y atención al público",
    description:
      "Locales adaptables para ventas temporales, showrooms, lanzamientos de marca o puntos de entrega.",
    examples: ["Pop-up store", "Showroom", "Local comercial"],
    howToPublish:
      "Sube fotos de la fachada e interior, describe el flujo peatonal de la zona y configura tus condiciones de contrato.",
  },
  stand: {
    image: "/editorial/cat-stand.jpg",
    imageAlt: "Módulo o stand de exhibición en espacio comercial",
    tagline: "Módulos para ferias, bazares y eventos",
    description:
      "Stands y módulos delimitados dentro de galerías, eventos o centros de alta circulación.",
    examples: ["Stand de feria", "Módulo en galería", "Punto de marca"],
    howToPublish:
      "Define medidas del módulo, disponibilidad por día o evento, y si incluye mobiliario o punto eléctrico.",
  },
  quincho: {
    image: "/editorial/hero-quincho.jpg",
    imageAlt: "Quincho equipado con parrilla, pérgola y mesa para compartir",
    tagline: "Asados, cumpleaños y encuentros privados",
    description:
      "Quinchos equipados con parrilla, mobiliario y áreas comunes para celebraciones familiares o corporativas.",
    examples: ["Quincho familiar", "Quincho con terraza", "Celebración de día"],
    howToPublish:
      "Indica aforo máximo, equipamiento de cocina/parrilla, horarios permitidos y reglas de convivencia.",
  },
  parcela_eventos: {
    image: "/editorial/cat-parcela.jpg",
    imageAlt: "Parcela y patio amplio acondicionado para eventos al aire libre",
    tagline: "Eventos al aire libre y jornadas de equipo",
    description:
      "Parcelas, jardines y recintos amplios para matrimonios, paseos de empresa, retiros y celebraciones.",
    examples: [
      "Parcela con piscina",
      "Centro de eventos",
      "Jornada corporativa",
    ],
    howToPublish:
      "Comparte fotografías de las áreas verdes, capacidad total, estacionamientos disponibles y reglas del recinto.",
  },
  inmuebles: {
    image: "/editorial/hero-casa.jpg",
    imageAlt: "Casa y departamento luminoso con terraza y áreas de estar",
    tagline: "Casas, departamentos, cabañas y habitaciones",
    description:
      "Arriendos residenciales por día o mes con información clara, ubicación referencial en el mapa y respaldo contractual.",
    examples: ["Casa", "Departamento", "Cabaña o habitación"],
    howToPublish:
      "Sube fotos de cada ambiente, indica dormitorios, estadía mínima y conecta Mercado Pago para recibir tus pagos.",
  },
};

export function getCategoryGuide(code: string): CategoryGuideInfo {
  return CATEGORY_GUIDES[code] ?? DEFAULT_GUIDE;
}
