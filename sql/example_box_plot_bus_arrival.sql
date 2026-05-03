-- Example: Bus stop arrival error distribution (box plot)
-- Uses the new five_d query type with route + stop API-driven selectors.
--
-- Front-end:
--   - chart type: BoxPlotChart
--   - 2 selectors: 公車路線 (selector_1 = route_uid), 站點 (selector_2 = stop_uid)
--   - Routes loaded from /api/v1/route, stops from /api/v1/route/<id>/stops
--
-- Back-end:
--   - query_type: two_selector_five_d
--   - SQL must return columns: x_axis, min, q1, median, q3, max
--
-- Replace the table and columns to match your dataset.

-- Step 1 — register component
INSERT INTO public.components (index, name)
VALUES ('bus_stop_arrival_error_box', '公車站點到站誤差分布')
ON CONFLICT (index) DO UPDATE
SET name = EXCLUDED.name;

-- Step 2 — register chart config (BoxPlotChart maps to types: ['BoxPlotChart'])
INSERT INTO public.component_charts (index, color, types, unit)
VALUES (
    'bus_stop_arrival_error_box',
    ARRAY['#5a9cf8', '#f88c5a'],
    ARRAY['BoxPlotChart'],
    '分鐘'
)
ON CONFLICT (index) DO UPDATE
SET
    color = EXCLUDED.color,
    types = EXCLUDED.types,
    unit = EXCLUDED.unit;

-- Step 3 — register the city query_charts row
DELETE FROM public.query_charts
WHERE index = 'bus_stop_arrival_error_box'
  AND city = 'taipei';

INSERT INTO public.query_charts (
    index,
    history_config,
    selector_config,
    map_config_ids,
    map_filter,
    time_from,
    time_to,
    update_freq,
    update_freq_unit,
    source,
    short_desc,
    long_desc,
    use_case,
    links,
    contributors,
    created_at,
    updated_at,
    query_type,
    query_chart,
    query_history,
    city
)
VALUES (
    'bus_stop_arrival_error_box',
    NULL,
    -- selector_config: two API-driven selectors. selector_2 cascades from selector_1.
    -- {selector_1.id} pulls the numeric route id from the selected route's raw API record.
    $sel${
      "selectors": [
        {
          "key": "selector_1",
          "label": "公車路線",
          "type": "search-select",
          "api_source": {
            "url": "/route",
            "value_field": "route_uid",
            "label_field": "route_name"
          }
        },
        {
          "key": "selector_2",
          "label": "站點",
          "type": "search-select",
          "depends_on": "selector_1",
          "api_source": {
            "url": "/route/{selector_1.id}/stops",
            "value_field": "stop_uid",
            "label_field": "stop_name"
          }
        }
      ]
    }$sel$::json,
    ARRAY[]::integer[],
    '{}'::json,
    'current',
    NULL,
    0,
    NULL,
    '公共運輸處',
    '依路線與站點顯示近 24 小時公車到站誤差的分布',
    '盒鬚圖呈現選定路線、站點下的到站誤差最小值、Q1、中位數、Q3、最大值，幫助辨認該站到站時間的離散程度與偏斜方向。',
    '公車調度與服務品質檢視',
    ARRAY['https://tdx.transportdata.tw/']::text[],
    ARRAY[]::text[],
    NOW(),
    NOW(),
    'two_selector_five_d',
    $query$
SELECT
    direction::text AS x_axis,
    MIN(EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0)::float                             AS min,
    percentile_cont(0.25) WITHIN GROUP (ORDER BY EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0)::float AS q1,
    percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0)::float AS median,
    percentile_cont(0.75) WITHIN GROUP (ORDER BY EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0)::float AS q3,
    MAX(EXTRACT(epoch FROM (actual_time - predicted_time)) / 60.0)::float                             AS max
FROM public.mv_bus_arrival_error_detail
WHERE route_uid = :selector_1
  AND stop_uid  = :selector_2
  AND actual_time >= NOW() - INTERVAL '24 hours'
GROUP BY direction
ORDER BY direction
$query$,
    NULL,
    'taipei'
);

-- Step 4 — attach to a dashboard
UPDATE public.dashboards
SET components = array_append(
        COALESCE(components, ARRAY[]::integer[]),
        (SELECT id FROM public.components WHERE index = 'bus_stop_arrival_error_box')
    ),
    updated_at = NOW()
WHERE index = '<DASHBOARD_INDEX>'
  AND NOT (
      COALESCE(components, ARRAY[]::integer[])
      @> ARRAY[(SELECT id FROM public.components WHERE index = 'bus_stop_arrival_error_box')]
  );
