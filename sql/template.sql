-- Taipei City Dashboard - Add Data / Component Template
-- Fill every <PLACEHOLDER> before running any SQL.
--
-- This file is a working checklist plus SQL skeleton.
-- Data rows live in the Dashboard DB.
-- Component metadata lives in the Manager DB.

-- Step 1 - Decide the data shape
-- Do what:
--   Choose whether this is chart-only data, map-only data, or both.
--   Decide the final Dashboard DB table name, time column, geometry column, and city scope.
--
-- Expected chart query outputs:
--   two_d:      x_axis, data
--   three_d:    x_axis, y_axis, data, icon
--   percent:    x_axis, y_axis, data, icon
--   time:       x_axis, y_axis, data
--   map_legend: name, type, icon, value

-- Step 2 - Create the DE DAG
-- Do what:
--   Add a folder under Taipei-City-Dashboard-DE/dags/<PROJECT_FOLDER>/<DAG_FOLDER>/.
--   Add job_config.json and <DAG_FOLDER>.py.
--   Copy the pattern from Taipei-City-Dashboard-DE/dags/tutorial/simple_template/.
--
-- Required job_config.json fields:
--   dag_infos.dag_id
--   dag_infos.schedule_interval
--   dag_infos.ready_data_db
--   dag_infos.ready_data_default_table
--   dag_infos.ready_data_history_table
--   dag_infos.load_behavior
--   data_infos.name_cn
--   data_infos.source
--   data_infos.source_type
--   data_infos.source_dept
--   data_infos.is_geometry

-- Step 3 - Implement ETL
-- Do what:
--   Extract source data.
--   Normalize column names and types.
--   Add data_time with timezone.
--   If geometry exists, convert it to wkb_geometry in EPSG:4326.
--   Save to Dashboard DB with save_dataframe_to_postgresql or save_geodataframe_to_postgresql.
--   Update dataset_info.lasttime_in_data.

-- Step 4 - Verify Dashboard DB data
-- Do what:
--   Run the final query directly against Dashboard DB.
--   Confirm returned column names match the selected query_type.
--   Confirm no null/invalid values break chart rendering.
--
-- Example:
-- SELECT x_axis, data
-- FROM public.<READY_DATA_TABLE>
-- LIMIT 20;

-- Step 5 - Register base component metadata in Manager DB
-- Do what:
--   Add one row to components.
--   Add one row to component_charts.
--   Add rows to component_maps only if this component has map layers.

INSERT INTO public.components (index, name)
VALUES ('<COMPONENT_INDEX>', '<COMPONENT_NAME>')
ON CONFLICT (index) DO UPDATE
SET name = EXCLUDED.name;

INSERT INTO public.component_charts (index, color, types, unit)
VALUES (
    '<COMPONENT_INDEX>',
    ARRAY['<#COLOR_1>', '<#COLOR_2>'],
    ARRAY['<CHART_TYPE_1>', '<CHART_TYPE_2>'],
    '<UNIT>'
)
ON CONFLICT (index) DO UPDATE
SET
    color = EXCLUDED.color,
    types = EXCLUDED.types,
    unit = EXCLUDED.unit;

-- Optional map layer metadata.
-- For local GeoJSON, put the file at:
--   Taipei-City-Dashboard-FE/public/mapData/<MAP_INDEX>.geojson
-- Then set source = 'geojson' and index = '<MAP_INDEX>'.
--
-- INSERT INTO public.component_maps
--     (index, title, type, source, size, icon, paint, property)
-- VALUES (
--     '<MAP_INDEX>',
--     '<MAP_TITLE>',
--     '<MAPBOX_LAYER_TYPE>',
--     'geojson',
--     NULL,
--     NULL,
--     '<MAPBOX_PAINT_JSON>'::json,
--     '<POPUP_PROPERTY_JSON_ARRAY>'::json
-- )
-- RETURNING id;

-- Step 6 - Register query_charts by city
-- Do what:
--   Add one query_charts row for each supported city.
--   query_chart must read from Dashboard DB tables and return the required output columns.
--   map_config_ids should contain component_maps.id values, or an empty array.

DELETE FROM public.query_charts
WHERE index = '<COMPONENT_INDEX>'
  AND city = '<taipei|metrotaipei>';

INSERT INTO public.query_charts (
    index,
    history_config,
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
    '<COMPONENT_INDEX>',
    NULL,
    ARRAY[]::integer[],
    '{}'::json,
    '<static|current|demo|relative_time>',
    NULL,
    <UPDATE_FREQ_OR_0>,
    '<minute|hour|day|month|NULL>',
    '<SOURCE_DEPARTMENT>',
    '<SHORT_DESC>',
    '<LONG_DESC>',
    '<USE_CASE>',
    ARRAY['<SOURCE_LINK>'],
    ARRAY['<CONTRIBUTOR_ID>'],
    NOW(),
    NOW(),
    '<two_d|three_d|percent|time|map_legend>',
    $query$
SELECT
    <X_AXIS_EXPR> AS x_axis,
    <DATA_EXPR> AS data
FROM public.<READY_DATA_TABLE>
$query$,
    NULL,
    '<taipei|metrotaipei>'
);

-- Step 7 - Attach component to a dashboard
-- Do what:
--   Append the new component id into dashboards.components.
--   Use the city dashboard index you want the component to appear in.

UPDATE public.dashboards
SET components = array_append(
        COALESCE(components, ARRAY[]::integer[]),
        (SELECT id FROM public.components WHERE index = '<COMPONENT_INDEX>')
    ),
    updated_at = NOW()
WHERE index = '<DASHBOARD_INDEX>'
  AND NOT (
      COALESCE(components, ARRAY[]::integer[])
      @> ARRAY[(SELECT id FROM public.components WHERE index = '<COMPONENT_INDEX>')]
  );

-- Step 8 - Verify API and frontend
-- Do what:
--   Test the component metadata:
--     GET /api/v1/component/<COMPONENT_ID>?city=<CITY>
--   Test chart data:
--     GET /api/v1/component/<COMPONENT_ID>/chart?city=<CITY>
--   If map exists, open map view and confirm /mapData/<MAP_INDEX>.geojson loads.
