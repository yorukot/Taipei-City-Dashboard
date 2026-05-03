# Route Reliability Endpoint

## Overview

`POST /api/v1/route/reliability` 會針對路線規劃結果中的每一段 leg 回傳可用的可靠度 metrics。

這個 endpoint 的責任是「查資料並整理成前端可判斷的數值」，不在後端輸出 `Green` / `Yellow` / `Red`，也不回傳舊版的 `status` / `label` / `route_status`。前端可以依照不同交通模式和 metric value 自行決定顏色、警示文字或圖表呈現。

目前支援：

- Bus：使用 `mv_bus_arrival_error_detail` 回傳近 24 小時公車到站誤差聚合資料。
- Rail / TRA：使用 `train_realtime`，用台鐵站點可靠度趨勢圖表相同概念，回傳同小時歷史準點/誤點 metrics。
- YouBike：使用雙北 YouBike availability tables，回傳同小時歷史平均可借車輛與可還空位。

其他模式目前回傳 `available: false`。

## Endpoint

```http
POST /api/v1/route/reliability
Content-Type: application/json
```

Route is registered under `/route`:

```text
/api/v1/route/reliability
```

The controller response still uses the backend standard top-level wrapper:

```json
{
  "status": "success",
  "data": {
    "legs": []
  }
}
```

`status` here is only the HTTP API wrapper status. Individual route legs do not contain reliability color/status.

## Request

```json
{
  "departure_time": "2026-05-03T08:30:00+08:00",
  "legs": [
    {
      "id": "bus-1",
      "mode": "BUS",
      "selector_key": "TPE810|TPE18177|0",
      "route_name": "敦化幹線",
      "stop_name": "榮總一",
      "direction": 0
    },
    {
      "id": "rail-1",
      "mode": "RAIL",
      "station_id": "1000",
      "train_type_code": "all",
      "start_time": "2026-05-03T08:30:00+08:00"
    },
    {
      "id": "bike-1",
      "mode": "YOUBIKE",
      "pickup_station_uid": "TPE500101001",
      "return_station_uid": "TPE500101001",
      "start_time": "2026-05-03T08:30:00+08:00"
    }
  ]
}
```

### Top-Level Fields

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `departure_time` | string | yes | Route departure time. Must be RFC3339, for example `2026-05-03T08:30:00+08:00`. |
| `legs` | array | yes | Route legs to analyze. Must contain at least one leg. |

### Common Leg Fields

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `id` | string | no | Client-side leg identifier. Echoed in response. |
| `mode` | string | yes | Transportation mode. Supported values include `BUS`, `RAIL`, `TRAIN`, `TRA`, `YOUBIKE`, `BICYCLE_RENTAL`, `BIKE_RENTAL`. |
| `start_time` | string | no | Leg-specific time. If omitted, backend uses `departure_time`. |
| `from_name` | string | no | Start station/stop name. Used as fallback matching input. |
| `to_name` | string | no | End station/stop name. Used as fallback matching input. |

## Response

Each leg returns:

| Name | Type | Description |
| --- | --- | --- |
| `id` | string | Echoed leg ID. |
| `mode` | string | Normalized mode. |
| `available` | boolean | Whether backend found enough data for this leg. |
| `reason` | string | Human-readable data availability note. |
| `metrics` | array | Raw metrics for frontend rendering and threshold calculation. Empty when unavailable. |
| `source` | string | Main table/materialized view used. |
| `match_quality` | string | Matching quality: `selector`, `exact`, `fallback`, or `unavailable`. |

Metric shape:

```json
{
  "key": "avg_available_rent_bikes",
  "label": "平均可借車輛",
  "value": 5.2,
  "unit": "輛"
}
```

Example response:

```json
{
  "status": "success",
  "data": {
    "legs": [
      {
        "id": "bus-1",
        "mode": "BUS",
        "available": true,
        "reason": "近 24 小時公車到站誤差資料共 17 筆",
        "metrics": [
          {
            "key": "avg_abs_arrival_error_minutes",
            "label": "平均到站誤差",
            "value": 4.8,
            "unit": "分鐘"
          },
          {
            "key": "on_time_count",
            "label": "準時班次",
            "value": 12,
            "unit": "筆"
          },
          {
            "key": "late_count",
            "label": "未準時班次",
            "value": 5,
            "unit": "筆"
          },
          {
            "key": "sample_count",
            "label": "樣本數",
            "value": 17,
            "unit": "筆"
          }
        ],
        "source": "mv_bus_arrival_error_detail",
        "match_quality": "selector"
      }
    ]
  }
}
```

## Bus

Bus reliability uses:

```text
public.mv_bus_arrival_error_detail
```

Relevant columns:

| Column | Meaning |
| --- | --- |
| `route_uid` | Bus route UID. |
| `route_name` | Route display name. |
| `stop_uid` | Stop UID. |
| `stop_name` | Stop display name. |
| `direction` | Route direction, usually `0` or `1`. |
| `actual_time` | Actual arrival timestamp. |
| `predicted_time` | Predicted arrival timestamp. |
| `error_minutes` | Existing error field in the materialized view. |
| `selector_key` | Route-stop-direction key used by existing bus chart selectors. |
| `selector_label` | Human-readable selector label. |

### Matching

Bus matching priority:

1. `selector_key`
2. `route_name` + `stop_name` + `direction`
3. `route_name` + `stop_name`
4. `route_name` + `direction`
5. `route_name`

`stop_name` falls back to `from_name`. Route and stop names also try `台` / `臺` variants. Route names additionally try removing `公車` and `路線`.

### Time Window

Bus follows the existing bus chart approach and uses the latest 24 hours in `mv_bus_arrival_error_detail`, based on the matched data's max `actual_time`:

```sql
matched.actual_time >= max(matched.actual_time) - interval '24 hours'
```

This makes the endpoint work with sample or delayed ETL data, because it does not assume the database's latest bus data is from the current wall-clock day.

### Bus Metrics

The backend computes signed arrival error as:

```sql
EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0
```

Positive value means actual arrival was later than prediction. Negative value means actual arrival was earlier than prediction.

| Key | Label | Unit | Calculation |
| --- | --- | --- | --- |
| `avg_abs_arrival_error_minutes` | 平均到站誤差 | 分鐘 | `avg(abs(actual_time - predicted_time))` in minutes. |
| `on_time_count` | 準時班次 | 筆 | `abs(error_minutes) <= 5`. |
| `late_count` | 未準時班次 | 筆 | `abs(error_minutes) > 5`. Includes too early and too late arrivals. |
| `early_over_5_count` | 提前 5 分以上 | 筆 | `signed_error_minutes < -5`. |
| `late_5_to_10_count` | 誤點 5-10 分 | 筆 | `signed_error_minutes > 5 AND signed_error_minutes <= 10`. |
| `late_over_10_count` | 誤點 10 分以上 | 筆 | `signed_error_minutes > 10`. |
| `sample_count` | 樣本數 | 筆 | Count of matched rows in latest 24-hour window. |

## Rail / TRA

Rail reliability uses:

```text
public.train_realtime
public.train_station
```

The calculation follows the same idea as the Taiwan Railway station reliability trend chart:

- Resolve station by `station_id`, `station_uid`, `from_name`, or `to_name`.
- Bucket time by Taipei local hour, for example `08:00`.
- Query `train_realtime` rows whose `update_time` is in the same hour bucket.
- Filter train type when `train_type_code` is provided and not `all`.

### Rail Metrics

| Key | Label | Unit | Calculation |
| --- | --- | --- | --- |
| `on_time_rate` | 準點率 | `%` | `delay_time < 5`. |
| `delay_rate` | 誤點率 | `%` | `delay_time >= 5 AND delay_time <= 10`. |
| `severe_delay_rate` | 嚴重誤點率 | `%` | `delay_time > 10`. |
| `sample_count` | 樣本數 | 筆 | Count of matched rows. |

## YouBike

YouBike reliability uses:

```text
public.ubike_availability_tpe
public.ubike_availability_new_tpe
public.ubike_station_tpe
public.ubike_station_new_tpe
```

The endpoint returns expected historical availability for the same Taipei local hour:

- Pickup station: use `pickup_station_uid`, then `station_uid`, then resolve by `pickup_station_name` or `from_name`.
- Return station: use `return_station_uid`, then resolve by `return_station_name` or `to_name`.
- Availability rows from old Taipei and New Taipei tables are unioned.
- Values are averaged for the same hour bucket as the leg start time.

### YouBike Metrics

| Key | Label | Unit | Calculation |
| --- | --- | --- | --- |
| `avg_available_rent_bikes` | 平均可借車輛 | 輛 | Average `available_rent_bikes` for pickup station and same hour. |
| `rent_sample_count` | 可借樣本數 | 筆 | Count of pickup availability rows. |
| `avg_available_return_bikes` | 平均可還空位 | 格 | Average `available_return_bikes` for return station and same hour. |
| `return_sample_count` | 可還樣本數 | 筆 | Count of return availability rows. |

## Unavailable Legs

When no data can be matched, the endpoint returns a leg with `available: false`:

```json
{
  "id": "unknown-1",
  "mode": "BUS",
  "available": false,
  "reason": "查無近 24 小時公車到站誤差資料",
  "metrics": [],
  "source": "mv_bus_arrival_error_detail",
  "match_quality": "unavailable"
}
```

Common unavailable cases:

- The mode is not implemented.
- Required station/route fields are missing.
- Name matching cannot find a station or bus route.
- The matched data has no samples for the target time window.
- Database connection is not initialized.

## Implementation Notes

- Backend implementation lives in `Taipei-City-Dashboard-BE/app/models/routeReliability.go`.
- Controller lives in `Taipei-City-Dashboard-BE/app/controllers/routeReliability.go`.
- Route registration lives in `Taipei-City-Dashboard-BE/app/routes/router.go`.
- Focused tests live in `Taipei-City-Dashboard-BE/app/models/routeReliability_test.go`.
- Metrics are rounded to one decimal place when they represent averages or rates.
- Count metrics are returned as numeric `value` fields for a consistent frontend metric shape.
- Frontend rendering is intentionally not part of this endpoint contract.
