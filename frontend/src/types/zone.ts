export interface Zone {
  id: number
  short_name: string
  long_name: string
  file_name: string
  zone_id_number: number
  safe_x: number
  safe_y: number
  safe_z: number
  min_level: number
  note: string
}

export interface SearchResult<T> {
  items: T[]
  total: number
}
