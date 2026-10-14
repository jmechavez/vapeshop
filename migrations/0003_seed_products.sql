INSERT INTO products
  (sku, name, brand, version, flavor, category, cost_price, selling_price, minimum_stock, active)
VALUES
  ('ELF-BC-5000', 'Elf Bar BC5000', 'Elf Bar', '5000 puffs', 'Blue Razz', 'Disposable', 220.00, 450.00, 5, TRUE),
  ('ELF-BC-5001', 'Elf Bar BC5000', 'Elf Bar', '5000 puffs', 'Watermelon', 'Disposable', 220.00, 450.00, 5, TRUE),
  ('RELX-POD-01', 'RELX Essential Pod', 'RELX', 'Essential', 'Mint', 'Pod', 180.00, 350.00, 10, TRUE),
  ('VOOPOO-DRAG', 'VooPoo Drag X', 'VooPoo', 'Drag X', '', 'Device', 900.00, 1500.00, 3, TRUE),
  ('COIL-MESH-01', 'Mesh Coil 0.6Ω', 'VooPoo', 'PNP', '', 'Coil', 60.00, 120.00, 20, TRUE)
ON CONFLICT (sku) DO NOTHING;

INSERT INTO inventory (product_id, current_stock)
SELECT id, 0 FROM products
ON CONFLICT (product_id) DO NOTHING;