INSERT INTO suppliers (name, contact_person, phone, email, address) VALUES
    ('Manila Vape Distributors Inc.',
     'Ramon Cruz',
     '0917-555-0101',
     'sales@manilavapedist.com',
     '123 Quiapo St., Manila'),

    ('Luzon Wholesale Supply Co.',
     'Grace Tan',
     '0918-555-0202',
     'orders@luzonwholesale.ph',
     '45 Session Rd., Baguio City'),

    ('SouthVape Trading',
     'Mark Reyes',
     '0919-555-0303',
     'contact@southvape.ph',
     '88 J.P. Laurel Ave., Davao City')
ON CONFLICT DO NOTHING;