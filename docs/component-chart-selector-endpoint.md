# Component Chart Data Endpoint

## Overview

`GET /api/v1/component/:id/chart` 會根據 `query_charts.query_chart` 取得 component chart data。

此 endpoint 原本支援：

- `city`
- `timefrom`
- `timeto`

目前新增支援：

- `selector_1`
- `selector_2`

`selector_1` 和 `selector_2` 是通用互動查詢參數。後端不解析它們的業務語意，只把它們安全地傳進 SQL。實際代表什麼由各 component 的 `query_chart` 決定，例如路線、方向、站點區間、行政區、測站、類別等。

## Request

```http
GET /api/v1/component/{id}/chart?city=taipei&timefrom=2026-05-01T00:00:00+08:00&timeto=2026-05-02T00:00:00+08:00&selector_1=A&selector_2=B
```

### Path Parameters

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | integer | yes | Component ID. |

### Query Parameters

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `city` | string | no | City value. Supported values are `taipei`, `metrotaipei`, or empty. Empty defaults to `taipei`. |
| `timefrom` | string | no | Start time. Format: `YYYY-MM-DDTHH:mm:ss+08:00`. Defaults to `1990-01-01T00:00:00+08:00`. |
| `timeto` | string | no | End time. Format: `YYYY-MM-DDTHH:mm:ss+08:00`. Defaults to current server time. |
| `selector_1` | string | conditionally | First generic selector value. Required when `query_type` is a `two_selector*` type, or when SQL references `:selector_1`. |
| `selector_2` | string | conditionally | Second generic selector value. Required when `query_type` is a `two_selector*` type, or when SQL references `:selector_2`. |

## Supported Query Types

Existing query types are still supported:

| `query_type` | Expected SQL Output Columns |
| --- | --- |
| `two_d` | `x_axis`, `data` |
| `three_d` | `x_axis`, `y_axis`, `data`, optional `icon` |
| `percent` | `x_axis`, `y_axis`, `data`, optional `icon` |
| `time` | `x_axis`, `y_axis`, `data` |
| `map_legend` | `name`, `type`, optional `icon`, optional `value` |

Two-selector query types are aliases that require both `selector_1` and `selector_2`:

| `query_type` | Base Parser | Use Case |
| --- | --- | --- |
| `two_selector` | `three_d` | Default two-selector chart data. |
| `two_selectors` | `three_d` | Alias of `two_selector`. |
| `two_selector_three_d` | `three_d` | Explicit three-dimensional output. |
| `two_selector_two_d` | `two_d` | Two-selector query returning `x_axis`, `data`. |
| `two_selector_percent` | `percent` | Two-selector query returning percent-style grouped data. |
| `two_selector_time` | `time` | Two-selector query returning time-series data. |
| `two_selector_map_legend` | `map_legend` | Two-selector query returning map legend data. |

For most dual-dropdown data, use `two_selector` unless the SQL output is clearly one of the other shapes.

## SQL Parameters

New SQL should use named placeholders:

```sql
:timefrom
:timeto
:selector_1
:selector_2
```

The backend binds these values with SQL named arguments. Do not build SQL by concatenating selector values.

Legacy SQL with exactly two `%s` placeholders is still supported for `timefrom` and `timeto` compatibility, but new selector-based SQL should use named placeholders.

## Example: Two-Selector Query

```sql
SELECT
    stop_name AS x_axis,
    metric_name AS y_axis,
    value AS data
FROM public.some_ready_data_table
WHERE data_time BETWEEN :timefrom::timestamptz AND :timeto::timestamptz
  AND route_key = :selector_1
  AND segment_key = :selector_2
ORDER BY stop_sequence;
```

Example `query_charts` values:

```sql
query_type = 'two_selector'
query_chart = $query$
SELECT
    stop_name AS x_axis,
    metric_name AS y_axis,
    value AS data
FROM public.some_ready_data_table
WHERE data_time BETWEEN :timefrom::timestamptz AND :timeto::timestamptz
  AND route_key = :selector_1
  AND segment_key = :selector_2
ORDER BY stop_sequence
$query$
```

Example request:

```http
GET /api/v1/component/123/chart?city=taipei&selector_1=route-a&selector_2=segment-b
```

## Response Shapes

### `two_d` / `two_selector_two_d`

```json
{
  "status": "success",
  "data": [
    {
      "data": [
        { "x": "A", "y": 10 },
        { "x": "B", "y": 20 }
      ]
    }
  ]
}
```

### `three_d` / `percent` / `two_selector`

```json
{
  "status": "success",
  "data": [
    {
      "name": "Metric A",
      "icon": "",
      "data": [10, 20]
    }
  ],
  "categories": ["A", "B"]
}
```

### `time` / `two_selector_time`

```json
{
  "status": "success",
  "data": [
    {
      "name": "Metric A",
      "data": [
        { "x": "2026-05-01T00:00:00+08:00", "y": 10 }
      ]
    }
  ]
}
```

### `map_legend` / `two_selector_map_legend`

```json
{
  "status": "success",
  "data": [
    {
      "name": "Layer A",
      "type": "line",
      "icon": "",
      "value": 0
    }
  ]
}
```

## Error Handling

| Status | Condition |
| --- | --- |
| `400` | Invalid component ID. |
| `400` | Invalid `city`. |
| `400` | Invalid `timefrom` or `timeto` format. |
| `400` | `query_type` is unsupported. |
| `400` | SQL or `query_type` requires `selector_1` / `selector_2`, but request does not provide it. |
| `404` | Component has no chart query or query type. |
| `500` | Database query or scan error. |

Missing selector example:

```json
{
  "status": "error",
  "message": "missing required chart selector: selector_1"
}
```

## Implementation Notes

- `selector_1` and `selector_2` are opaque strings at the chart endpoint layer.
- Per-component selector meaning belongs in `query_chart`.
- A `two_selector*` query type always requires both selector values, even if SQL does not reference them directly.
- A normal query type only requires a selector when its SQL references that selector placeholder.
- Use `two_selector` as the default new type for dual-dropdown chart data.
