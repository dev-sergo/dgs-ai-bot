# Структура данных Dooglys (нормализовано, PII вырезан)

## ABC — abc  (строк: 20)
```json
{"category_name": "Тестирую", "name": "Плескавица", "quantity": 68.0, "cost_price": 13549.88}
```

## Активные заказы — active-orders  (строк: 20)
```json
{"region_name": "Москва", "sale_point_name": "ТС ПИоТ", "kassir": "REDACTED", "source": "Касса", "nomer": "TC1", "tip": "В заведении", "tip_oplaty": "Наличные", "total_cost": 115.0, "status": "Подтвержден", "first_name": null, "telefon_pokupatelya": null, "in_time": "Нет", "date_delivery": "16.06.2026 18:43:13", "date_created": "16.06.2026 18:23:13", "col": "ПодробнееДетали"}
```

## Внесения и выплаты — cash-income-outcome  (строк: 1)
```json
{"name": "ТС ПИоТ", "close_date": "19 июн. 2026 г.", "tip": "Внесение", "sum": 1.0, "cashier_name": "REDACTED", "prichina": "Открытие смены"}
```

## Наличные — cash-on-hand  (строк: 1)
```json
{"name": "ТС ПИоТ", "sell": 0.0, "payback": 0.0, "income": 1.0, "outcome": 0.0, "total": 1.0}
```

## Категории — categories  (строк: 20)
```json
{"name": "Американо", "quantity": 8.0, "amount": 0.0, "profit": 0.0}
```

## Клиенты — clients  (строк: 20)
```json
{"last_name": "REDACTED", "sex": "Муж.", "email": "redacted@example.com", "phone": "+70000000000", "registration_date": "18 апр. 2019 г.", "profile_status": "Зарегистрирована", "birth_date": null, "crm_status": null}
```

## Ожидаемая прибыль — expected-profit  (строк: 20)
```json
{"date": "2025-01-20", "real_count": 1.0, "sum_all": 20.0, "cost_price": 0.0, "profit": 20.0, "percent": 0.0}
```

## Балансы по уровням — level-balance  (строк: 20)
```json
{"title": "asd", "date": "2026-06", "balance": 501.32}
```

## Минимальные остатки — minimum-stock  (строк: 1)
```json
{"col": null}
```

## Часы заказов — order-payment-hour  (строк: 16)
```json
{"hour": 1.0, "quantity": 2.0, "sales_sum": 178.87, "items_sum": 178.87}
```

## Время выполнения — order-processing-time  (строк: 1)
```json
{"sale_point_name": "Выкса", "not_confirmed_time": "1 минута", "confirmed_time": "9 минут", "cooking_time": "1 минута", "ready_time": "45 минут", "sent_time": "28 минут", "processing_time": "38 минут"}
```

## Заказы — orders  (строк: 20)
```json
{"number": "RU1", "istochnik": "Касса", "kassir": "REDACTED", "pokupatel": null, "torgovaya_tochka": "ТС ПИоТ", "open": "18 июн. 19:37", "dostavlen": null, "zavershen": null, "paid": 203.0, "discount_value": 0.0, "status": "Заказ завершен", "tip_zakaza": "Розничная продажа", "tip_oplaty": "Наличные", "dostavka_ko_vremeni": null, "prichina_vozvrata": null, "fio": null, "adres": null, "koordinaty": null, "col": "ПодробнееДетали"}
```

## Чеки — paycheck  (строк: 20)
```json
{"number": 138.0, "zakaza": "RU1", "shift_number": 19.0, "cashier_name": "REDACTED", "terminal_name": "ТСПИоТ SUNMI", "tip_cheka": "Продажа", "close": "18 июн. 19:37", "tip_oplaty": "Наличные", "paid": 203.0, "sell_discount": 0.0, "profit": 202.77, "col": "ПодробнееДетали"}
```

## Выручка — payment  (строк: 20)
```json
{"date": "2026-06-18", "kol_vo_chekov": 4.0, "return_count": 0.0, "return_sum": 0.0, "sum_card": 257.87, "sum_cash": 252.87, "onlayn": 0.0, "sbp": 0.0, "sum_all": 510.74, "sredniy_chek": 127.68, "col": "Детали"}
```

## Персонал — personnel  (строк: 6)
```json
{"name": "REDACTED", "earnings": 340.0, "profit": 67.0, "checks_count": 2.0, "average_check": 170.0}
```

## Отчёт по движению — product-flow  (строк: 1)
```json
{"sklad": null}
```

## Товары — products  (строк: 20)
```json
{"name": "Bon-Aqua 0.26 мл.", "artikul": 113.0, "quantity": 0.0, "quantity_with_discount": 0.0, "quantity_without_discount": 0.0, "amount": 0.0, "amount_with_discount": 0.0, "amount_without_discount": 0.0, "profit": 0.0, "profit_with_discount": 0.0, "profit_without_discount": 0.0, "discount_sum": 0.0}
```

## Динамика закупочных цен — purchase-price-dynamics  (строк: 1)
```json
{"produkt": null}
```

## Продажи на карте — sales-on-map  (строк: 1)
```json
{"name": null}
```

## Источники заказов — source-order  (строк: 5)
```json
{"source": "Касса", "delivery_quantity": 1789.47, "delivery_total": "11 017,58 ₽ 81,04%", "delivery_profit": "1 518,55 ₽ 68,09%", "hold_quantity": 1359.09, "hold_total": "3 387,87 ₽ 52,51%", "hold_profit": "1 391,14 ₽ 45,61%", "retail_quantity": 113100.0, "retail_total": "27 821,58 ₽ 100,00%", "retail_profit": "-8 561,40 ₽ -856 140,00%", "pickup_quantity": 1664.0, "pickup_total": "6 859,28 ₽ 80,71%", "pickup_profit": "805,47 ₽ 55,93%", "quantity": 159.0, "total": 49086.31, "profit": -4846.24}
```

## Товары по акции — special-products  (строк: 20)
```json
{"special_name": 10.0, "product_name": "Четыре сыра СБОРНАЯ", "total_cost": 90.0, "discount": 10.0, "quantity": 1.0}
```

## Акции — special  (строк: 9)
```json
{"name": 10.0, "orders_count": 14.0, "orders_amount_without_discount": 14179.21, "orders_amount_discount": 1418.04, "orders_amount_with_discount": 13661.17}
```

## Остатки по складам — stock  (строк: 1)
```json
{"product_name": null}
```

## Поступления (склад) — warehouse-document-supply  (строк: 20)
```json
{"number": "ПОСТ-51", "is_register": "Не проведен", "registration_date": null, "incoming_document_number": null, "warehouse_id": "dslslkfjsdf", "contractor_id": "222-дубль", "nds": null, "nds_1": "Без НДС", "invoice": null, "waybill": null, "comment": null}
```

## Отчёт по списаниям — write-off-report  (строк: 1)
```json
{"dokument": null}
```

## coupon_coupon — coupon_coupon  (строк: 20)
```json
{"col": null, "code": "DYRKOW2", "deystvitelen_s": null, "deystvitelen_po": null, "data_poslednego_ispolzovaniya": null, "yavlyaetsya_odnorazovym": "Нетмногократное применение", "kolichestvo_primeneniy": null, "col_1": "Удалить Редактировать"}
```

## device_terminal_archive — device_terminal_archive  (строк: 20)
```json
{"name": "Новый терминал", "kod_aktivacii": null, "tip_terminala": "Касса-ритейл", "torgovaya_tochka": null, "sklad": "пустой склад", "last_login": "02.10.2023 17:32:14", "date_activated": "05.10.2023 14:48:47", "col": "Восстановить"}
```

## device_terminal_index — device_terminal_index  (строк: 20)
```json
{"name": "ТС ПИоТ", "kod_aktivacii": null, "tip_terminala": "Касса-ритейл", "torgovaya_tochka": "ТС ПИоТ", "sklad": null, "last_login": "19.06.2026 12:47:44", "date_activated": "-", "col": "Редактировать Посмотреть настройки Отвязать устройство"}
```

## device_terminal_info_39053269-81be-4c5f-9434-74be90d12e81 — device_terminal_info_39053269-81be-4c5f-9434-74be90d12e81  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_39ee1334-ac91-4c93-ad88-8899f0804725 — device_terminal_info_39ee1334-ac91-4c93-ad88-8899f0804725  (строк: 3)
```json
{"col": 1.0, "nazvanie": "Фискальный регистратор АТОЛ(5)", "model": "FiscalRegistrarAtol5", "ffd": null}
```

## device_terminal_info_5b85fcaa-6116-46ab-85b9-15cba816c77c — device_terminal_info_5b85fcaa-6116-46ab-85b9-15cba816c77c  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_68c09b74-8c24-4ac0-9baa-17de459068a2 — device_terminal_info_68c09b74-8c24-4ac0-9baa-17de459068a2  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_7308e8d4-3675-4cdf-9e06-62fcc96d47d4 — device_terminal_info_7308e8d4-3675-4cdf-9e06-62fcc96d47d4  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_79521208-3823-4273-abed-ec7bd2e1fc3c — device_terminal_info_79521208-3823-4273-abed-ec7bd2e1fc3c  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_84277e0f-991f-4b8c-9400-c8f085837be8 — device_terminal_info_84277e0f-991f-4b8c-9400-c8f085837be8  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_8ac09369-160e-4f67-b728-737efe49c5f7 — device_terminal_info_8ac09369-160e-4f67-b728-737efe49c5f7  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_8cde472d-88b0-4888-bad2-ec72b5ecd7c3 — device_terminal_info_8cde472d-88b0-4888-bad2-ec72b5ecd7c3  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_94b72499-3f20-4588-98cd-2f83f8fdec61 — device_terminal_info_94b72499-3f20-4588-98cd-2f83f8fdec61  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_94f734cb-c691-4fe9-9509-5601d414f2c0 — device_terminal_info_94f734cb-c691-4fe9-9509-5601d414f2c0  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_c5c5b129-fb05-47f4-9c84-c698d50def0d — device_terminal_info_c5c5b129-fb05-47f4-9c84-c698d50def0d  (строк: 1)
```json
{"col": null}
```

## device_terminal_info_c952beac-53f8-4822-b1a6-3c8a5dc90ee8 — device_terminal_info_c952beac-53f8-4822-b1a6-3c8a5dc90ee8  (строк: 1)
```json
{"col": null}
```

## extra-field_list_product — extra-field_list_product  (строк: 1)
```json
{"title": null}
```

## extra-field_list_sale_point — extra-field_list_sale_point  (строк: 1)
```json
{"title": null}
```

## journal_entry_index — journal_entry_index  (строк: 20)
```json
{"col": 1.0, "occurred_on": "19 июн. 2026 г., 12:08:04", "sobytie": "Предприятие / Вход в систему", "informaciya": "Пользователь google_test вошел в систему", "polzovatel": "google_test"}
```

## loyalty_card — loyalty_card  (строк: 20)
```json
{"num": 0.0, "state": "Зарегистрирована", "customer_id": "REDACTED", "mode": "Бонусная", "col": "Редактировать Заблокировать Перенести бонусы"}
```

## message-templates_template — message-templates_template  (строк: 12)
```json
{"alias": "Лояльность - Подтверждение электронной почтыОтправляется при подтверждении почты пользователем из ЛК лояльности", "send_email": "redacted@example.com", "send_sms": "Нет", "col": "Редактировать"}
```

## Комбо — nomenclature_combo_index  (строк: 2)
```json
{"col": null, "name": "Test combo (шт) Повар", "product_category_id": "пицца", "col_1": "Посмотреть"}
```

## nomenclature_manufacture-place_index — nomenclature_manufacture-place_index  (строк: 5)
```json
{"name": "Кухня", "col": "Редактировать Удалить"}
```

## Материалы и ингредиенты — nomenclature_material_index  (строк: 20)
```json
{"col": null, "name": "123 (шт)", "product_category_id": "Пакет чай", "col_1": "Посмотреть"}
```

## Модификаторы — nomenclature_modifier  (строк: 20)
```json
{"col": null, "name": "33 см (шт) Спец.Мод.Произв.ПоварКБЖУСвоё", "product_category_id": "Модификаторы", "ispolzuetsya": 0.0, "price": 2.0, "col_1": "Редактировать"}
```

## Полуфабрикаты — nomenclature_prepack  (строк: 8)
```json
{"col": null, "name": "asdasd (шт) Спец.", "product_category_id": "123ewfef", "col_1": "Посмотреть"}
```

## nomenclature_product-yellow-price — nomenclature_product-yellow-price  (строк: 9)
```json
{"short_name": "выкса", "kolichestvo_cennikov": 6.0, "col": "Редактировать"}
```

## Товары и услуги — nomenclature_product_index  (строк: 20)
```json
{"col": null, "name": "111 (шт) Спец.ПоварКБЖУ", "product_category_id": "СНЕКИ", "price": 110.0, "nacenka": "-15 785,47/-99,31 %", "col_1": "Посмотреть"}
```

## notification_campaign_index — notification_campaign_index  (строк: 5)
```json
{"nazvanie": "test", "data_sozdaniya": "7 мая 2026 г., 16:31:08", "data_obnovleniya": "7 мая 2026 г., 16:31:08", "col": "Редактировать Удалить"}
```

## notification_item — notification_item  (строк: 7)
```json
{"title": "ewrgwerge", "status": "Черновик", "publish_date": null, "prochitano": 0.0, "col": "Копировать Редактировать Удалить"}
```

## notification_item_archive — notification_item_archive  (строк: 13)
```json
{"title": "Test", "status": "Опубликовано", "publish_date": "4 дек. 2019 г., 18:26:03", "prochitano": 1.0, "col": "Восстановить"}
```

## notification_item_view_37df2048-b804-4c61-b9b2-6941dbfb2383 — notification_item_view_37df2048-b804-4c61-b9b2-6941dbfb2383  (строк: 1)
```json
{"user_id": "qatestProd", "read_time": "5 июн. 2026 г., 20:08:03"}
```

## notification_item_view_73985d63-acb3-4d90-9856-ede477140b2e — notification_item_view_73985d63-acb3-4d90-9856-ede477140b2e  (строк: 1)
```json
{"user_id": "qatestProd", "read_time": "2 июл. 2025 г., 12:08:56"}
```

## notification_item_view_8d28488a-880e-4703-a443-557774492f90 — notification_item_view_8d28488a-880e-4703-a443-557774492f90  (строк: 1)
```json
{"user_id": "qatestProd", "read_time": "3 июн. 2025 г., 11:52:05"}
```

## notification_item_view_a060ef43-2b01-4119-8f52-34b1a643f238 — notification_item_view_a060ef43-2b01-4119-8f52-34b1a643f238  (строк: 1)
```json
{"user_id": "qatestProd", "read_time": "2 июл. 2025 г., 12:08:56"}
```

## notification_item_view_e25125f1-a6d8-4d08-afea-d94b8c0aa456 — notification_item_view_e25125f1-a6d8-4d08-afea-d94b8c0aa456  (строк: 2)
```json
{"user_id": "Google Google", "read_time": "24 янв. 2025 г., 13:24:40"}
```

## planning_production-plan — planning_production-plan  (строк: 15)
```json
{"name": 123.0, "telefon": "+70000000000", "kolichestvo_stolov": 4.0, "col": "Настроить"}
```

## report_abc — report_abc  (строк: 10)
```json
{"category_name": "Упаковка", "name": "Салфетка", "quantity": 15.0, "cost_price": 3.45}
```

## report_active-orders — report_active-orders  (строк: 20)
```json
{"region_name": "Москва", "sale_point_name": "ТС ПИоТ", "kassir": "REDACTED", "source": "Касса", "nomer": "TC1", "tip": "В заведении", "tip_oplaty": "Наличные", "total_cost": 115.0, "status": "Подтвержден", "first_name": null, "telefon_pokupatelya": null, "in_time": "Нет", "date_delivery": "16.06.2026 18:43:13", "date_created": "16.06.2026 18:23:13", "col": "ПодробнееДетали"}
```

## report_bonuses — report_bonuses  (строк: 2)
```json
{"data": "2026-06-03", "zachisleniya": 2.0, "zachisleniya_summa": 2000.0, "spisaniya": 0.0, "spisaniya_summa": 0.0, "pokupki_urovnya": 0.0, "pokupki_urovnya_summa": 0.0, "sgoraniya": 0.0, "sgoraniya_summa": 0.0, "bez_kart": 1.0, "bez_kart_summa": 100.0}
```

## report_cash-income-outcome — report_cash-income-outcome  (строк: 1)
```json
{"name": "ТС ПИоТ", "close_date": "19 июн. 2026 г.", "tip": "Внесение", "sum": 1.0, "cashier_name": "REDACTED", "prichina": "Открытие смены"}
```

## report_cash-on-hand — report_cash-on-hand  (строк: 1)
```json
{"name": "ТС ПИоТ", "sell": 0.0, "payback": 0.0, "income": 1.0, "outcome": 0.0, "total": 1.0}
```

## report_categories — report_categories  (строк: 6)
```json
{"name": "Второе", "quantity": 3.0, "amount": 180.0, "profit": -153.0}
```

## report_clients — report_clients  (строк: 20)
```json
{"last_name": "REDACTED", "sex": "Муж.", "email": "redacted@example.com", "phone": "+70000000000", "registration_date": "18 апр. 2019 г.", "profile_status": "Зарегистрирована", "birth_date": null, "crm_status": null}
```

## report_customer-segmentation — report_customer-segmentation  (строк: 1)
```json
{"fio": "Фамилия Имя Отчество", "telefon": "+70000000000", "birth_date": "2022-12-15", "rfm_gruppa": null, "region": "Выкса осн", "torgovaya_tochka": "Выкса", "quantity": 2.0, "amount": 1000.0, "avg_amount": 500.0, "tip_oplaty": "Картой, Наличные", "tip_zakaza": "Доставка, Самовывоз", "istochnik": "Касса", "bonus_quantity": 16015.12, "kupon": null, "tovary": "Салфетка, Длинное название самой большой пиццы в мире", "kategorii_tovarov": "Упаковка, Пицца", "registration_date": "2022-11-11"}
```

## report_dangerous-operations — report_dangerous-operations  (строк: 1)
```json
{"transactions_count": null}
```

## report_expected-profit — report_expected-profit  (строк: 3)
```json
{"date": "2026-06-15", "real_count": 1.0, "sum_all": 416.0, "cost_price": 0.23, "profit": 415.77, "percent": 180769.57}
```

## report_level-balance — report_level-balance  (строк: 20)
```json
{"title": "asd", "date": "2026-06", "balance": 501.32}
```

## report_level-orders — report_level-orders  (строк: 5)
```json
{"title": "Основная группа", "date": "2026-06", "order_count": 2.0, "sredniy_chek": 500.0, "vyruchka": 1000.0}
```

## report_minimum-stock — report_minimum-stock  (строк: 1)
```json
{"col": null}
```

## report_order-payment-hour — report_order-payment-hour  (строк: 11)
```json
{"hour": 1.0, "quantity": 1.0, "sales_sum": 49.87, "items_sum": 49.87}
```

## report_order-processing-time-detail — report_order-processing-time-detail  (строк: 1)
```json
{"order_day": null}
```

## report_order-processing-time — report_order-processing-time  (строк: 1)
```json
{"sale_point_name": null}
```

## report_orders — report_orders  (строк: 1)
```json
{"number": null}
```

## report_paycheck — report_paycheck  (строк: 1)
```json
{"number": null}
```

## report_payment — report_payment  (строк: 8)
```json
{"date": "2026-06-18", "kol_vo_chekov": 4.0, "return_count": 0.0, "return_sum": 0.0, "sum_card": 257.87, "sum_cash": 252.87, "onlayn": 0.0, "sbp": 0.0, "sum_all": 510.74, "sredniy_chek": 127.68, "col": "Детали"}
```

## report_personnel — report_personnel  (строк: 2)
```json
{"name": "REDACTED", "earnings": 2471.48, "profit": 1970.26, "checks_count": 14.0, "average_check": 176.53}
```

## report_product-flow — report_product-flow  (строк: 1)
```json
{"sklad": null}
```

## report_products — report_products  (строк: 8)
```json
{"name": "Биолимонад \"Мед и травы\" 0.33", "artikul": null, "quantity": 1.0, "quantity_with_discount": 0.0, "quantity_without_discount": 1.0, "amount": 150.0, "amount_with_discount": 0.0, "amount_without_discount": 150.0, "profit": 95.0, "profit_with_discount": 0.0, "profit_without_discount": 95.0, "discount_sum": 0.0}
```

## report_purchase-price-dynamics — report_purchase-price-dynamics  (строк: 1)
```json
{"produkt": null}
```

## report_rfm-analyze — report_rfm-analyze  (строк: 3)
```json
{"fio": "Dmitry", "gruppa": "В зоне потери", "rfm_indeks": 133.0, "last_purchase_days": 203.0, "purchase_quantity": 15.0, "purchase_amount": 4926.0}
```

## report_sales-on-map — report_sales-on-map  (строк: 1)
```json
{"name": null}
```

## report_special-products — report_special-products  (строк: 1)
```json
{"special_name": null}
```

## report_special — report_special  (строк: 1)
```json
{"name": null}
```

## report_stock — report_stock  (строк: 1)
```json
{"product_name": null}
```

## report_suspect-orders — report_suspect-orders  (строк: 1)
```json
{"nomer_zakaza": null}
```

## report_type-order — report_type-order  (строк: 11)
```json
{"shift_date": "18 июн. 2026 г.", "region_name": "Выкса осн", "sale_point_name": "Выкса", "quantity": 4.0, "sales_sum": 407.74, "roznichnye_prodazhi_kol_vo": 4.0, "roznichnye_prodazhi_nalichnye": 149.87, "roznichnye_prodazhi_karta": 257.87, "roznichnye_prodazhi_onlayn": 0.0, "roznichnye_prodazhi_sbp": 0.0, "v_zavedenii_kol_vo": 0.0, "v_zavedenii_nalichnye": 0.0, "v_zavedenii_karta": 0.0, "v_zavedenii_onlayn": 0.0, "v_zavedenii_sbp": 0.0, "samovyvoz_kol_vo": 0.0, "samovyvoz_nalichnye": 0.0, "samovyvoz_karta": 0.0, "samovyvoz_onlayn": 0.0, "samovyvoz_sbp": 0.0, "dostavka_kol_vo": 0.0, "dostavka_nalichnye": 0.0, "dostavka_karta": 0.0, "dostavka_onlayn": 0.0, "dostavka_sbp": 0.0}
```

## report_warehouse-document-supply — report_warehouse-document-supply  (строк: 20)
```json
{"number": "ПОСТ-51", "is_register": "Не проведен", "registration_date": null, "incoming_document_number": null, "warehouse_id": "dslslkfjsdf", "contractor_id": "222-дубль", "nds": null, "nds_1": "Без НДС", "invoice": null, "waybill": null, "comment": null}
```

## report_write-off-report — report_write-off-report  (строк: 1)
```json
{"dokument": null}
```

## sales_customer — sales_customer  (строк: 20)
```json
{"col": null, "last_name": "REDACTED", "telefon": "+70000000000", "email": null, "karty": "нет активных карт", "loyalty_level_id": "Основная группа", "col_1": "Редактировать Удалить"}
```

## sales_order_view_0a395522-6812-4125-8f86-6c9494eacb63 — sales_order_view_0a395522-6812-4125-8f86-6c9494eacb63  (строк: 2)
```json
{"nazvanie": "Каштановый раф", "cena": 259.0, "skidka": 77.7, "kolichestvo": 1.0, "kommentariy": null, "itogo": 181.3}
```

## sales_order_view_2008e57a-83df-4c3f-8468-95bebed1f891 — sales_order_view_2008e57a-83df-4c3f-8468-95bebed1f891  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_32c3526f-2715-469f-b9c6-ef2523703eea — sales_order_view_32c3526f-2715-469f-b9c6-ef2523703eea  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_33b870c3-a704-4ce0-b1c6-1f89bc47ad9e — sales_order_view_33b870c3-a704-4ce0-b1c6-1f89bc47ad9e  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_37a2dc39-cdf6-40f4-94cb-902b31854b26 — sales_order_view_37a2dc39-cdf6-40f4-94cb-902b31854b26  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_43ecc901-4b06-4aa5-8d6f-efbd71d5cf6d — sales_order_view_43ecc901-4b06-4aa5-8d6f-efbd71d5cf6d  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_494762c8-2a09-4f1a-83d9-f6c5dd95f464 — sales_order_view_494762c8-2a09-4f1a-83d9-f6c5dd95f464  (строк: 1)
```json
{"number": 0.0, "shift_number": 0.0, "cashier_name": "REDACTED", "terminal_name": "ТС ПИоТ", "tip_cheka": "Продажа", "close": "16 июн. 18:23", "tip_oplaty": "Наличные", "paid": 115.0, "sell_discount": 0.0, "profit": 114.77, "col": "Подробнее"}
```

## sales_order_view_4b16973a-bcbf-4751-b605-6df49c12d4c8 — sales_order_view_4b16973a-bcbf-4751-b605-6df49c12d4c8  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_4ef80967-2e85-4379-ba5c-ce092fb5c538 — sales_order_view_4ef80967-2e85-4379-ba5c-ce092fb5c538  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_59a7e875-72c7-4cbc-ba47-af058b0da79e — sales_order_view_59a7e875-72c7-4cbc-ba47-af058b0da79e  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_5d943415-bd17-4479-b172-0f15f5ec540f — sales_order_view_5d943415-bd17-4479-b172-0f15f5ec540f  (строк: 1)
```json
{"number": 1.0, "shift_number": 102.0, "cashier_name": "REDACTED", "terminal_name": "WEB", "tip_cheka": "Продажа", "close": "01 июн. 16:50", "tip_oplaty": "Картой", "paid": 150.0, "sell_discount": 0.0, "profit": 94.77, "col": "Подробнее"}
```

## sales_order_view_5e3adb3a-afa5-49d7-a8e7-b7f3b766327b — sales_order_view_5e3adb3a-afa5-49d7-a8e7-b7f3b766327b  (строк: 2)
```json
{"nazvanie": "Котлета", "cena": 50.0, "skidka": 0.0, "kolichestvo": 1.0, "kommentariy": null, "itogo": 50.0}
```

## sales_order_view_843b637c-8f70-4d27-8b6d-043d86c444ac — sales_order_view_843b637c-8f70-4d27-8b6d-043d86c444ac  (строк: 3)
```json
{"nazvanie": "Спагетти", "cena": 60.0, "skidka": 0.0, "kolichestvo": 1.0, "kommentariy": null, "itogo": 60.0}
```

## sales_order_view_8613fda1-d483-4096-9bfb-a1fa7ea37228 — sales_order_view_8613fda1-d483-4096-9bfb-a1fa7ea37228  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_8cf2d20e-bb8b-4691-8130-bc4e69a5d1e8 — sales_order_view_8cf2d20e-bb8b-4691-8130-bc4e69a5d1e8  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_b1651a09-6c1d-4f06-9142-ad7d9ce3bda1 — sales_order_view_b1651a09-6c1d-4f06-9142-ad7d9ce3bda1  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_b2c87b63-6a7c-4277-920e-18a2cf1409f2 — sales_order_view_b2c87b63-6a7c-4277-920e-18a2cf1409f2  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_beb67abd-f566-46bb-be38-9e22fabf5056 — sales_order_view_beb67abd-f566-46bb-be38-9e22fabf5056  (строк: 2)
```json
{"nazvanie": "Биолимонад \"Мед и травы\" 0.33", "cena": 150.0, "skidka": 0.0, "kolichestvo": 1.0, "kommentariy": null, "itogo": 150.0}
```

## sales_order_view_bfab0ebc-2bca-4cb4-b163-137f539db87e — sales_order_view_bfab0ebc-2bca-4cb4-b163-137f539db87e  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## sales_order_view_c47bbcd4-2d0f-475c-8d28-6cfd42e2526c — sales_order_view_c47bbcd4-2d0f-475c-8d28-6cfd42e2526c  (строк: 2)
```json
{"nazvanie": "Салфетка комплектация", "cena": 0.0, "skidka": 0.0, "kolichestvo": "1 шт", "kommentariy": null, "itogo": 0.0}
```

## special_special — special_special  (строк: 19)
```json
{"col": null, "col_1": null, "rule_name": "На тип заказа", "kommentariy": null, "usloviya": null, "created_at": "4 июн. 2026 г., 14:55:55", "sozdal": "REDACTED", "obnovil": "REDACTED", "aktivna": "Да", "col_2": "Редактировать Удалить Копировать Отключить"}
```

## stop-list_default — stop-list_default  (строк: 15)
```json
{"col": 1.0, "name": 123.0, "kol_vo_tovarov": 0.0}
```

## structure_locality — structure_locality  (строк: 8)
```json
{"name": "603000, Нижегородская обл, г Нижний Новгород", "kratkoe_nazvanie": "НН", "radius_poiska_adresov_km": "REDACTED", "data_sozdaniya": "16 нояб. 2020 г., 15:04:29", "col": "Редактировать Удалить"}
```

## structure_locality_archive — structure_locality_archive  (строк: 9)
```json
{"name": "101000, г Москва", "kratkoe_nazvanie": "die", "radius_poiska_adresov_km": "REDACTED", "data_sozdaniya": "1 сент. 2022 г., 13:39:44", "col": "Восстановить"}
```

## structure_organization — structure_organization  (строк: 8)
```json
{"yuridicheskoe_nazvanie": "asd", "tip_sistemy_nalogooblozheniya": "Общая система - ОСН", "inn": null, "adres": "REDACTED", "col": "Редактировать Удалить"}
```

## structure_organization_archive — structure_organization_archive  (строк: 3)
```json
{"yuridicheskoe_nazvanie": "wer", "tip_sistemy_nalogooblozheniya": null, "inn": null, "adres": "REDACTED", "col": "Восстановить"}
```

## structure_post — structure_post  (строк: 13)
```json
{"name": "Промо менеджер", "col": "Редактировать Удалить"}
```

## Торговые точки — structure_sale-point  (строк: 15)
```json
{"name": 123.0, "region": "Выкса осн", "telefon": "+70000000000", "kolichestvo_stolov": 4.0, "col": "Редактировать Удалить"}
```

## structure_user — structure_user  (строк: 20)
```json
{"col": 1.0, "fio": "REDACTED", "login": "alexey", "dolzhnost": "Директор", "sostoyanie": "Активен", "email": null, "pin_kod": "0000", "col_1": "Редактировать Удалить"}
```

## structure_warehouses — structure_warehouses  (строк: 9)
```json
{"nazvanie": "ТС ПИоТ", "col": "Редактировать Удалить"}
```

## structure_warehouses_archive — structure_warehouses_archive  (строк: 2)
```json
{"nazvanie": "Test1", "col": "Восстановить"}
```

## telephony_operator_index — telephony_operator_index  (строк: 2)
```json
{"operator": "Манго Офис", "podklyuchenie": "Подключено", "col": "Редактировать Удалить"}
```

## terminal-menu_menu — terminal-menu_menu  (строк: 20)
```json
{"col": 1.0, "name": "fdsafdsa_Копия2", "created_at": "14 сент. 2018 г., 13:13:39", "col_1": "Конструктор меню Копировать Удалить"}
```

## trade-recommendation_recommendation — trade-recommendation_recommendation  (строк: 6)
```json
{"name": "Предложить воду при покупке пиццы", "aktivna": "Нет", "col": "Редактировать Удалить"}
```

## warehouse_contractor — warehouse_contractor  (строк: 5)
```json
{"name": "222-дубль", "phone_number": null, "fax": null, "email": null, "legal_address": null, "col": "Редактировать Удалить"}
```

## warehouse_document — warehouse_document  (строк: 20)
```json
{"number": "СПИС-25", "is_register": "Не проведен", "registration_date": null, "nomer_vhodyaschego_dokumenta": null, "kommentariy": null, "col": "Редактировать Копировать Удалить"}
```

## warehouse_minimum-stock — warehouse_minimum-stock  (строк: 1)
```json
{"col": null}
```
