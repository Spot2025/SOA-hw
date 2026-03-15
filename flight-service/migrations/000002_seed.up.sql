-- Пример рейса для тестирования
INSERT INTO flights (flight_number, airline, origin, destination, departure_time, arrival_time, total_seats, available_seats, price, status)
SELECT 'SU1234', 'Aeroflot', 'SVO', 'LED', '2026-04-01 10:00:00+00', '2026-04-01 11:30:00+00', 100, 100, '5000.00', 'SCHEDULED'
WHERE NOT EXISTS (SELECT 1 FROM flights WHERE flight_number = 'SU1234' AND departure_time::date = '2026-04-01');
