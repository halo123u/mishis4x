-- passwords get hashed
-- this might not actually work for login
INSERT INTO user (username, status, password)
VALUES ('john_doe', 'active', 'mysecretpassword');
INSERT INTO user (username, status, password)
VALUES ('jane_doe', 'active', 'mysecretpassword2');