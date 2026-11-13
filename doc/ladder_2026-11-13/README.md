# The pattern ladder (2026-11-13)

Track B of `doc/requests/foundation_2026-11-13.md`: read every github.com/code-heim repo (a reading list only,
`doc/sources/code-heim.md`), re-research each pattern against today's Go and its authority, map it onto calcbench,
and merge the result into one build plan for toolbench. Read-only research: no pattern code is written here.

| File | What it is |
|---|---|
| `BRIEF.md` | The two rules, the row format, the tier definitions and the split given to the readers |
| `R1.md` | socket_io_emitter, go_2 .. go_25 |
| `R2.md` | go_26 .. go_49 |
| `R3.md` | go_50 .. go_70 |
| `R4.md` | go_71 .. go_85 |
| `LADDER.md` | The merge: the ladder, standard patterns not in code-heim, package map, API list, calcbench use, build waves, outdated, skipped, decisions |

## Repos (GitHub API, 2026-11-13)

81 found, 81 cloned (`git clone --depth 1` into `_reference/code-heim/<name>`, gitignored), 0 missing. Numbers
1, 16, 22, 29 and 84 do not exist in the org. Licence per the GitHub API: Apache-2.0 for 78, MIT for
go_7_go_decorator and go_8_redis_lock, none declared for go_55_grpc. Last push: 2023-11-27 (go_2) to
2025-04-23 (socket_io_emitter).

| Repo | Last push | Licence |
|---|---|---|
| socket_io_emitter | 2025-04-23 | Apache-2.0 |
| go_2_gorm_association_one_to_many | 2023-11-27 | Apache-2.0 |
| go_3_gorm_many_to_many | 2023-11-28 | Apache-2.0 |
| go_4_gorm_scopes | 2023-11-28 | Apache-2.0 |
| go_5_go_rss_feed | 2023-11-28 | Apache-2.0 |
| go_6_gorm_filters | 2023-11-28 | Apache-2.0 |
| go_7_go_decorator | 2023-12-07 | MIT |
| go_8_redis_lock | 2023-12-07 | MIT |
| go_9_gin_gorm_pagination | 2023-12-14 | Apache-2.0 |
| go_10_gorm_serializer | 2024-01-03 | Apache-2.0 |
| go_11_cache_sync_map | 2024-01-05 | Apache-2.0 |
| go_12_gorilla_websocket | 2024-01-08 | Apache-2.0 |
| go_13_gorm_custom_serializer | 2024-01-15 | Apache-2.0 |
| go_14_builder_pattern | 2024-01-15 | Apache-2.0 |
| go_15_graphql_mysql | 2024-01-17 | Apache-2.0 |
| go_17_env_variables | 2024-03-31 | Apache-2.0 |
| go_18_worker_pool | 2024-01-29 | Apache-2.0 |
| go_19_custom_logger | 2024-01-31 | Apache-2.0 |
| go_20_mutexes_and_confinement | 2024-02-05 | Apache-2.0 |
| go_21_goroutines_pipeline | 2024-02-08 | Apache-2.0 |
| go_23_gin_html_json_response | 2024-02-14 | Apache-2.0 |
| go_24_unbuffered_buffered_channels | 2024-02-19 | Apache-2.0 |
| go_25_fan_out_fan_in | 2024-02-22 | Apache-2.0 |
| go_26_slog | 2024-03-04 | Apache-2.0 |
| go_27_panic_recover | 2024-03-04 | Apache-2.0 |
| go_28_htmx | 2024-03-12 | Apache-2.0 |
| go_30_gin_auth | 2024-03-26 | Apache-2.0 |
| go_31_yaml_config | 2024-04-01 | Apache-2.0 |
| go_32_http_error_code | 2024-04-01 | Apache-2.0 |
| go_33_endless_scrolling | 2024-04-24 | Apache-2.0 |
| go_34_gorm_preload_vs_joins | 2024-04-24 | Apache-2.0 |
| go_35_gorm_polymorphism | 2024-05-01 | Apache-2.0 |
| go_36_gin_jwt | 2024-05-08 | Apache-2.0 |
| go_37_background_work | 2024-05-14 | Apache-2.0 |
| go_38_csv | 2024-05-21 | Apache-2.0 |
| go_39_excel | 2024-05-28 | Apache-2.0 |
| go_40_interfaces_generics | 2024-06-04 | Apache-2.0 |
| go_41_dependency_injection | 2024-06-11 | Apache-2.0 |
| go_42_faktory | 2024-06-18 | Apache-2.0 |
| go_43_net_http_middleware | 2024-07-03 | Apache-2.0 |
| go_44_kafka | 2024-07-03 | Apache-2.0 |
| go_45_memcache | 2024-07-09 | Apache-2.0 |
| go_46_elasticsearch | 2024-07-16 | Apache-2.0 |
| go_47_email | 2024-07-24 | Apache-2.0 |
| go_48_llm_cli | 2024-07-30 | Apache-2.0 |
| go_49_task_scheduling | 2024-08-06 | Apache-2.0 |
| go_50_task_scheduling_part_2 | 2024-08-13 | Apache-2.0 |
| go_51_context | 2024-08-27 | Apache-2.0 |
| go_52_protobuf | 2024-08-27 | Apache-2.0 |
| go_53_http_testing | 2024-09-03 | Apache-2.0 |
| go_54_sync_pool | 2024-09-08 | Apache-2.0 |
| go_55_grpc | 2024-09-16 | none |
| go_56_generator | 2024-09-23 | Apache-2.0 |
| go_57_build_tags | 2024-09-29 | Apache-2.0 |
| go_58_rabbitmq | 2024-10-07 | Apache-2.0 |
| go_59_pdf | 2024-10-13 | Apache-2.0 |
| go_60_iter | 2024-10-20 | Apache-2.0 |
| go_61_err_group | 2024-10-28 | Apache-2.0 |
| go_62_ordone_tee_channel | 2024-11-08 | Apache-2.0 |
| go_63_cobra_cli | 2024-11-13 | Apache-2.0 |
| go_64_multipart_upload | 2024-11-19 | Apache-2.0 |
| go_65_mongo | 2024-11-27 | Apache-2.0 |
| go_66_tunny | 2024-12-04 | Apache-2.0 |
| go_67_conc | 2024-12-10 | Apache-2.0 |
| go_68_asynq | 2024-12-16 | Apache-2.0 |
| go_69_bloom_filter | 2024-12-24 | Apache-2.0 |
| go_70_must_pattern | 2024-12-30 | Apache-2.0 |
| go_71_sqlx | 2025-01-08 | Apache-2.0 |
| go_72_viper_config | 2025-01-14 | Apache-2.0 |
| go_73_lambda | 2025-01-21 | Apache-2.0 |
| go_74_pgx | 2025-01-28 | Apache-2.0 |
| go_75_coder_websocket | 2025-02-04 | Apache-2.0 |
| go_76_aws_s3 | 2025-02-12 | Apache-2.0 |
| go_77_rate_limit | 2025-02-18 | Apache-2.0 |
| go_78_resty | 2025-02-26 | Apache-2.0 |
| go_79_loki_grafana | 2025-03-05 | Apache-2.0 |
| go_80_tcp_udp_sockets | 2025-03-12 | Apache-2.0 |
| go_81_loki_grafana_alloy | 2025-03-19 | Apache-2.0 |
| go_82_rate_limiting_revised | 2025-03-26 | Apache-2.0 |
| go_83_http_testing_2 | 2025-04-01 | Apache-2.0 |
| go_85_first_class_functions_closures | 2025-04-16 | Apache-2.0 |
